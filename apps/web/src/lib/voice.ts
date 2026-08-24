import type { Room as LiveKitRoom } from "livekit-client";
import { api } from "./api";

export interface VoiceStateHandlers {
  connected: (connected: boolean) => void;
  activeSpeaker: (identity: string | null) => void;
  error: (message: string) => void;
}

export class VoiceController {
  private room: LiveKitRoom | null = null;

  constructor(private readonly handlers: VoiceStateHandlers) {}

  async enableMicrophone(roomId: string) {
    try {
      if (!this.room) {
        const token = await api.voiceToken(roomId);
        if (!token.enabled || !token.url || !token.accessToken) {
          this.handlers.error(token.reason ?? "语音服务当前不可用");
          return false;
        }
        const { ConnectionState, Room, RoomEvent, Track } = await import("livekit-client");
        const room = new Room({ adaptiveStream: true, dynacast: true });
        room.on(RoomEvent.ConnectionStateChanged, (state) => this.handlers.connected(state === ConnectionState.Connected));
        room.on(RoomEvent.ActiveSpeakersChanged, (speakers) => this.handlers.activeSpeaker(speakers[0]?.identity ?? null));
        room.on(RoomEvent.TrackSubscribed, (track) => {
          if (track.kind === Track.Kind.Audio) track.attach();
        });
        room.on(RoomEvent.TrackUnsubscribed, (track) => {
          for (const element of track.detach()) element.remove();
        });
        room.on(RoomEvent.Disconnected, () => this.handlers.connected(false));
        await room.connect(token.url, token.accessToken);
        this.room = room;
      }
      await this.room.localParticipant.setMicrophoneEnabled(true);
      return true;
    } catch (reason) {
      this.handlers.error(reason instanceof Error ? reason.message : "无法开启麦克风");
      this.handlers.connected(false);
      return false;
    }
  }

  async disableMicrophone() {
    if (this.room) await this.room.localParticipant.setMicrophoneEnabled(false);
  }

  async microphones(requestPermissions = false) {
    const { Room } = await import("livekit-client");
    return Room.getLocalDevices("audioinput", requestPermissions);
  }

  async switchMicrophone(deviceId: string) {
    if (!this.room) return false;
    return this.room.switchActiveDevice("audioinput", deviceId, true);
  }

  disconnect() {
    this.room?.disconnect();
    this.room = null;
    this.handlers.connected(false);
    this.handlers.activeSpeaker(null);
  }
}
