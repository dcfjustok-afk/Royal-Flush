import type { Room as LiveKitRoom } from "livekit-client";
import { api, isRoomMembershipRevokedClose, isSessionRevokedClose, isVoiceConnectionReplacedClose, type VoiceToken } from "./api";

export type VoiceTransport = "livekit" | "webrtc";

export interface VoiceStateHandlers {
  connected: (connected: boolean) => void;
  activeSpeaker: (identity: string | null) => void;
  transport: (transport: VoiceTransport | null) => void;
  error: (message: string) => void;
}

interface VoicePeer {
  userId: string;
  nickname: string;
}

interface VoiceServerEvent {
  type: "voice.peers" | "voice.peer_joined" | "voice.peer_left" | "voice.signal" | "voice.error";
  userId?: string;
  nickname?: string;
  fromUserId?: string;
  signalType?: "voice.description" | "voice.candidate";
  payload?: RTCSessionDescriptionInit | RTCIceCandidateInit | null;
  peers?: VoicePeer[];
  message?: string;
}

interface PeerState {
  connection: RTCPeerConnection;
  makingOffer: boolean;
  ignoreOffer: boolean;
  settingRemoteAnswer: boolean;
  polite: boolean;
}

interface MeterState {
  analyser: AnalyserNode;
  samples: Uint8Array<ArrayBuffer>;
  source: MediaStreamAudioSourceNode;
}

export function isPoliteVoicePeer(localUserId: string, remoteUserId: string) {
  return localUserId.localeCompare(remoteUserId) > 0;
}

function microphoneError(reason: unknown) {
  if (reason instanceof DOMException && reason.name === "NotAllowedError") return "请允许浏览器使用麦克风";
  if (reason instanceof DOMException && reason.name === "NotFoundError") return "没有找到可用的麦克风";
  if (reason instanceof DOMException && reason.name === "NotReadableError") return "麦克风正被其他应用占用";
  return reason instanceof Error ? reason.message : "无法开启麦克风";
}

class MeshVoiceSession {
  private socket: WebSocket | null = null;
  private localStream: MediaStream | null = null;
  private readonly peers = new Map<string, PeerState>();
  private readonly audioElements = new Map<string, HTMLAudioElement>();
  private readonly meters = new Map<string, MeterState>();
  private audioContext: AudioContext | null = null;
  private meterTimer = 0;
  private reconnectTimer = 0;
  private reconnectAttempt = 0;
  private shouldReconnect = false;
  private signalingQueue: Promise<void> = Promise.resolve();
  private socketUrl = "";
  private localUserId = "";

  constructor(
    private readonly handlers: VoiceStateHandlers,
    private readonly iceServers: RTCIceServer[],
  ) {}

  async connect(socketUrl: string, localUserId: string) {
    if (!("RTCPeerConnection" in window) || !navigator.mediaDevices?.getUserMedia) {
      throw new Error("当前浏览器不支持桌内语音")
    }
    this.socketUrl = socketUrl;
    this.localUserId = localUserId;
    this.shouldReconnect = true;
    this.localStream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      video: false,
    });
    this.monitor(localUserId, this.localStream);
    this.startMeter();
    this.openSocket();
  }

  async setMicrophoneEnabled(enabled: boolean) {
    for (const track of this.localStream?.getAudioTracks() ?? []) track.enabled = enabled;
  }

  async switchMicrophone(deviceId: string) {
    const nextStream = await navigator.mediaDevices.getUserMedia({
      audio: { deviceId: { exact: deviceId }, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      video: false,
    });
    const nextTrack = nextStream.getAudioTracks()[0];
    if (!nextTrack) {
      nextStream.getTracks().forEach((track) => track.stop());
      return false;
    }
    const enabled = this.localStream?.getAudioTracks()[0]?.enabled ?? true;
    nextTrack.enabled = enabled;
    await Promise.all([...this.peers.values()].map(async ({ connection }) => {
      const sender = connection.getSenders().find((candidate) => candidate.track?.kind === "audio");
      if (sender) await sender.replaceTrack(nextTrack);
    }));
    this.localStream?.getTracks().forEach((track) => track.stop());
    this.stopMeter(this.localUserId);
    this.localStream = nextStream;
    this.monitor(this.localUserId, nextStream);
    return true;
  }

  disconnect() {
    this.shouldReconnect = false;
    window.clearTimeout(this.reconnectTimer);
    this.socket?.close(1000, "voice disabled");
    this.socket = null;
    for (const userId of [...this.peers.keys()]) this.removePeer(userId);
    this.localStream?.getTracks().forEach((track) => track.stop());
    this.localStream = null;
    this.stopMeter(this.localUserId);
    window.clearInterval(this.meterTimer);
    this.meterTimer = 0;
    void this.audioContext?.close();
    this.audioContext = null;
    this.handlers.connected(false);
    this.handlers.activeSpeaker(null);
  }

  private openSocket() {
    if (!this.shouldReconnect || this.socket?.readyState === WebSocket.OPEN || this.socket?.readyState === WebSocket.CONNECTING) return;
    const socket = new WebSocket(this.socketUrl);
    this.socket = socket;
    socket.addEventListener("open", () => {
      if (this.socket !== socket) return;
      this.reconnectAttempt = 0;
      this.handlers.connected(true);
    });
    socket.addEventListener("message", (message) => {
      this.signalingQueue = this.signalingQueue
        .then(() => this.handleEvent(JSON.parse(String(message.data)) as VoiceServerEvent))
        .catch((reason) => this.handlers.error(microphoneError(reason)));
    });
    socket.addEventListener("close", (event) => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.handlers.connected(false);
      for (const userId of [...this.peers.keys()]) this.removePeer(userId);
      if (isSessionRevokedClose(event.code)) {
        this.shouldReconnect = false;
        this.handlers.error("登录状态已失效，请重新登录");
        return;
      }
      if (isVoiceConnectionReplacedClose(event.code)) {
        this.shouldReconnect = false;
        this.handlers.error("语音已在另一个标签页开启");
        return;
      }
      if (isRoomMembershipRevokedClose(event.code)) {
        this.shouldReconnect = false;
        this.handlers.error("你已离开或被移出这个房间");
        return;
      }
      if (!this.shouldReconnect) return;
      const delay = Math.min(500 * 2 ** this.reconnectAttempt++, 5000);
      this.reconnectTimer = window.setTimeout(() => this.openSocket(), delay);
    });
    socket.addEventListener("error", () => this.handlers.error("语音信令暂时不可用，正在重连"));
  }

  private async handleEvent(event: VoiceServerEvent) {
    if (event.type === "voice.error") {
      this.handlers.error(event.message ?? "语音信令失败");
      return;
    }
    if (event.type === "voice.peers") {
      for (const peer of event.peers ?? []) this.ensurePeer(peer.userId);
      return;
    }
    if (event.type === "voice.peer_joined" && event.userId) {
      this.ensurePeer(event.userId);
      return;
    }
    if (event.type === "voice.peer_left" && event.userId) {
      this.removePeer(event.userId);
      return;
    }
    if (event.type !== "voice.signal" || !event.fromUserId || !event.signalType || event.payload === undefined) return;
    const state = this.ensurePeer(event.fromUserId);
    if (event.signalType === "voice.description") {
      const description = event.payload as RTCSessionDescriptionInit;
      const readyForOffer = !state.makingOffer && (state.connection.signalingState === "stable" || state.settingRemoteAnswer);
      const offerCollision = description.type === "offer" && !readyForOffer;
      state.ignoreOffer = !state.polite && offerCollision;
      if (state.ignoreOffer) return;
      state.settingRemoteAnswer = description.type === "answer";
      await state.connection.setRemoteDescription(description);
      state.settingRemoteAnswer = false;
      if (description.type === "offer") {
        await state.connection.setLocalDescription();
        this.sendSignal(event.fromUserId, "voice.description", state.connection.localDescription);
      }
      return;
    }
    try {
      await state.connection.addIceCandidate(event.payload as RTCIceCandidateInit | null);
    } catch (reason) {
      if (!state.ignoreOffer) throw reason;
    }
  }

  private ensurePeer(userId: string) {
    const existing = this.peers.get(userId);
    if (existing) return existing;
    const connection = new RTCPeerConnection({ iceServers: this.iceServers, iceCandidatePoolSize: 2 });
    const state: PeerState = {
      connection,
      makingOffer: false,
      ignoreOffer: false,
      settingRemoteAnswer: false,
      polite: isPoliteVoicePeer(this.localUserId, userId),
    };
    this.peers.set(userId, state);
    for (const track of this.localStream?.getTracks() ?? []) connection.addTrack(track, this.localStream!);
    connection.addEventListener("negotiationneeded", async () => {
      try {
        state.makingOffer = true;
        await connection.setLocalDescription();
        this.sendSignal(userId, "voice.description", connection.localDescription);
      } catch (reason) {
        this.handlers.error(microphoneError(reason));
      } finally {
        state.makingOffer = false;
      }
    });
    connection.addEventListener("icecandidate", ({ candidate }) => {
      if (candidate) this.sendSignal(userId, "voice.candidate", candidate.toJSON());
    });
    connection.addEventListener("track", ({ streams, track }) => {
      const stream = streams[0] ?? new MediaStream([track]);
      this.attachRemoteAudio(userId, stream);
    });
    connection.addEventListener("connectionstatechange", () => {
      if (connection.connectionState === "failed") connection.restartIce();
      if (connection.connectionState === "closed") this.removePeer(userId);
    });
    return state;
  }

  private sendSignal(targetUserId: string, type: "voice.description" | "voice.candidate", payload: unknown) {
    if (this.socket?.readyState !== WebSocket.OPEN || !payload) return;
    this.socket.send(JSON.stringify({ type, targetUserId, payload }));
  }

  private attachRemoteAudio(userId: string, stream: MediaStream) {
    let audio = this.audioElements.get(userId);
    if (!audio) {
      audio = document.createElement("audio");
      audio.autoplay = true;
      audio.setAttribute("playsinline", "");
      audio.hidden = true;
      audio.dataset.voiceUserId = userId;
      document.body.append(audio);
      this.audioElements.set(userId, audio);
    }
    audio.srcObject = stream;
    void audio.play().catch(() => this.handlers.error("请再次点击麦克风以允许播放桌内语音"));
    this.monitor(userId, stream);
  }

  private removePeer(userId: string) {
    const state = this.peers.get(userId);
    if (state) {
      state.connection.ontrack = null;
      state.connection.close();
      this.peers.delete(userId);
    }
    const audio = this.audioElements.get(userId);
    if (audio) {
      audio.srcObject = null;
      audio.remove();
      this.audioElements.delete(userId);
    }
    this.stopMeter(userId);
  }

  private monitor(userId: string, stream: MediaStream) {
    this.stopMeter(userId);
    const AudioContextConstructor = window.AudioContext;
    if (!AudioContextConstructor) return;
    this.audioContext ??= new AudioContextConstructor();
    void this.audioContext.resume();
    const analyser = this.audioContext.createAnalyser();
    analyser.fftSize = 256;
    analyser.smoothingTimeConstant = 0.72;
    const source = this.audioContext.createMediaStreamSource(stream);
    source.connect(analyser);
    this.meters.set(userId, { analyser, source, samples: new Uint8Array(analyser.fftSize) });
  }

  private stopMeter(userId: string) {
    const meter = this.meters.get(userId);
    meter?.source.disconnect();
    meter?.analyser.disconnect();
    this.meters.delete(userId);
  }

  private startMeter() {
    if (this.meterTimer) return;
    this.meterTimer = window.setInterval(() => {
      let loudest: string | null = null;
      let loudestLevel = 0.035;
      for (const [userId, meter] of this.meters) {
        meter.analyser.getByteTimeDomainData(meter.samples);
        let energy = 0;
        for (const sample of meter.samples) {
          const value = (sample - 128) / 128;
          energy += value * value;
        }
        const level = Math.sqrt(energy / meter.samples.length);
        if (level > loudestLevel) {
          loudest = userId;
          loudestLevel = level;
        }
      }
      this.handlers.activeSpeaker(loudest);
    }, 180);
  }
}

export class VoiceController {
  private liveKitRoom: LiveKitRoom | null = null;
  private readonly liveKitAudioElements = new Set<HTMLMediaElement>();
  private mesh: MeshVoiceSession | null = null;

  constructor(private readonly handlers: VoiceStateHandlers) {}

  async enableMicrophone(roomId: string, userId: string) {
    try {
      if (this.liveKitRoom) {
        await this.liveKitRoom.localParticipant.setMicrophoneEnabled(true);
        return true;
      }
      if (this.mesh) {
        await this.mesh.setMicrophoneEnabled(true);
        return true;
      }
      const token = await api.voiceToken(roomId);
      if (!token.enabled) {
        this.handlers.error(token.reason ?? "语音服务当前不可用");
        return false;
      }
      if (token.transport === "webrtc") {
        await this.connectMesh(roomId, userId, token);
      } else {
        await this.connectLiveKit(token);
      }
      return true;
    } catch (reason) {
      this.handlers.error(microphoneError(reason));
      this.handlers.connected(false);
      this.disconnect();
      return false;
    }
  }

  async disableMicrophone() {
    if (this.liveKitRoom) await this.liveKitRoom.localParticipant.setMicrophoneEnabled(false);
    if (this.mesh) await this.mesh.setMicrophoneEnabled(false);
  }

  async microphones(requestPermissions = false) {
    if (requestPermissions) {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
      stream.getTracks().forEach((track) => track.stop());
    }
    return (await navigator.mediaDevices.enumerateDevices()).filter((device) => device.kind === "audioinput");
  }

  async switchMicrophone(deviceId: string) {
    if (this.liveKitRoom) return this.liveKitRoom.switchActiveDevice("audioinput", deviceId, true);
    if (this.mesh) return this.mesh.switchMicrophone(deviceId);
    return false;
  }

  disconnect() {
    this.liveKitRoom?.disconnect();
    this.liveKitRoom = null;
    for (const element of this.liveKitAudioElements) element.remove();
    this.liveKitAudioElements.clear();
    this.mesh?.disconnect();
    this.mesh = null;
    this.handlers.transport(null);
    this.handlers.connected(false);
    this.handlers.activeSpeaker(null);
  }

  private async connectLiveKit(token: VoiceToken) {
    if (!token.url || !token.accessToken) throw new Error("LiveKit 语音配置不完整");
    const { ConnectionState, Room, RoomEvent, Track } = await import("livekit-client");
    const room = new Room({ adaptiveStream: true, dynacast: true });
    room.on(RoomEvent.ConnectionStateChanged, (state) => this.handlers.connected(state === ConnectionState.Connected));
    room.on(RoomEvent.ActiveSpeakersChanged, (speakers) => this.handlers.activeSpeaker(speakers[0]?.identity ?? null));
    room.on(RoomEvent.TrackSubscribed, (track) => {
      if (track.kind !== Track.Kind.Audio) return;
      const element = track.attach();
      element.hidden = true;
      document.body.append(element);
      this.liveKitAudioElements.add(element);
    });
    room.on(RoomEvent.TrackUnsubscribed, (track) => {
      for (const element of track.detach()) {
        element.remove();
        this.liveKitAudioElements.delete(element);
      }
    });
    room.on(RoomEvent.Reconnecting, () => this.handlers.connected(false));
    room.on(RoomEvent.Reconnected, () => this.handlers.connected(true));
    room.on(RoomEvent.Disconnected, () => this.handlers.connected(false));
    this.liveKitRoom = room;
    await room.connect(token.url, token.accessToken);
    await room.localParticipant.setMicrophoneEnabled(true);
    this.handlers.transport("livekit");
  }

  private async connectMesh(roomId: string, userId: string, token: VoiceToken) {
    const iceServers: RTCIceServer[] = (token.iceServers ?? []).map((server) => ({
      urls: server.urls,
      username: server.username,
      credential: server.credential,
    }));
    const mesh = new MeshVoiceSession(this.handlers, iceServers);
    await mesh.connect(api.voiceWebSocketUrl(roomId), userId);
    this.mesh = mesh;
    this.handlers.transport("webrtc");
  }
}
