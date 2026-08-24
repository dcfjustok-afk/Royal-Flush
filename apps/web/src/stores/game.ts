import type { ChipDenomination, RoomConfig, ScoreLedgerEntry, TableSnapshot } from "@royal-flush/contracts";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { emptyRoomConfig, emptySnapshot } from "@/data/empty";
import { api, ApiError, apiMode, isSessionRevokedClose, type AccountUser } from "@/lib/api";
import { VoiceController, type VoiceTransport } from "@/lib/voice";

type Message = { id: string; type: string; text: string; at: string };
export type RealtimeConnectionState = "offline" | "connecting" | "connected" | "reconnecting";

export function reconnectDelay(attempt: number) {
  return Math.min(250 * 2 ** Math.max(0, attempt), 2000);
}

export const useGameStore = defineStore("game", () => {
  const accountPoints = ref<number | null>(null);
  const currentUser = ref<AccountUser | null>(null);
  const activeRoomId = ref("");
  const ledger = ref<ScoreLedgerEntry[]>([]);
  const roomConfig = ref<RoomConfig>(structuredClone(emptyRoomConfig));
  const snapshot = ref<TableSnapshot>(structuredClone(emptySnapshot));
  const messages = ref<Message[]>([]);
  const microphoneEnabled = ref(false);
  const voiceConnected = ref(false);
  const voiceTransport = ref<VoiceTransport | null>(null);
  const voiceError = ref("");
  const activeSpeakerId = ref<string | null>(null);
  const microphones = ref<Array<{ deviceId: string; label: string }>>([]);
  const selectedMicrophoneId = ref("");
  const voiceBusy = ref(false);
  const backendOnline = ref(false);
  const connectionState = ref<RealtimeConnectionState>("offline");
  const connectionError = ref("");
  const roomLoading = ref(false);
  const commandPending = ref(false);
  const lastScoreAdditionAt = ref(0);
  let eventSocket: WebSocket | null = null;
  let eventRoomId = "";
  let shouldReconnect = false;
  let reconnectAttempt = 0;
  let reconnectTimer = 0;
  const voice = new VoiceController({
    connected: (connected) => (voiceConnected.value = connected),
    activeSpeaker: (identity) => {
      activeSpeakerId.value = identity;
      for (const player of snapshot.value.players) player.isSpeaking = player.id === identity;
    },
    transport: (transport) => (voiceTransport.value = transport),
    error: (message) => (voiceError.value = message),
  });

  const localPlayer = computed(() => snapshot.value.players.find((player) => player.isLocal));
  const activePlayers = computed(() => snapshot.value.players.filter((player) => player.status !== "away" && player.status !== "disconnected").length);

  async function probeBackend() {
    if (!apiMode) return;
    try {
      await api.health();
      backendOnline.value = true;
    } catch {
      backendOnline.value = false;
    }
  }

  async function refreshAccount() {
    if (!apiMode || !backendOnline.value) return;
    try {
      const [me, scoreLedger] = await Promise.all([api.me(), api.scoreLedger()]);
      currentUser.value = me.user;
      accountPoints.value = me.balance;
      activeRoomId.value = me.activeRoomId ?? "";
      ledger.value = scoreLedger.entries;
      if (me.activeRoomId) await loadRoom(me.activeRoomId).catch(() => undefined);
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 401) {
        clearAccount();
        return;
      }
      throw reason;
    }
  }

  function acceptSnapshot(next: TableSnapshot) {
    snapshot.value = structuredClone(next);
    roomConfig.value = structuredClone(next.config ?? { ...roomConfig.value, name: next.roomName, chipDenominations: [...next.allowedChipDenominations] });
    if (next.messages) {
      messages.value = next.messages.map((message) => ({
        id: message.id,
        type: message.type,
        text: message.text,
        at: new Date(message.createdAt).toLocaleTimeString("zh-CN", { hour12: false }),
      }));
    }
    const me = next.players.find((player) => player.isLocal);
    if (me) {
      accountPoints.value = me.accountPoints;
      currentUser.value = currentUser.value
        ? { ...currentUser.value, id: me.id, nickname: me.name }
        : { id: me.id, phone: "", nickname: me.name, permissions: {}, banned: false, createdAt: "" };
      if (me.isMuted && microphoneEnabled.value) {
        void voice.disableMicrophone();
        microphoneEnabled.value = false;
        voiceError.value = "房主已将你的麦克风静音";
      }
    }
  }

  async function createRoom(config: RoomConfig) {
    updateRoomConfig(config);
    requireBackend();
    const created = await api.createRoom(config);
    acceptSnapshot(created.snapshot);
    return { id: created.id, code: created.code };
  }

  async function loadRoom(roomId: string) {
    requireBackend();
    roomLoading.value = true;
    try {
      const next = await api.roomSnapshot(roomId);
      acceptSnapshot(next);
      return next;
    } finally {
      roomLoading.value = false;
    }
  }

  async function joinRoom(idOrCode: string, seat: number) {
    const next = await api.joinRoom(idOrCode, seat);
    acceptSnapshot(next);
    backendOnline.value = true;
    return next;
  }

  function requireBackend() {
    if (!apiMode || !backendOnline.value) throw new Error("服务暂时不可用，请稍后重试");
  }

  async function sendCommand(type: string, payload: Record<string, unknown> = {}) {
    if (commandPending.value) throw new Error("上一项操作仍在处理中");
    const roomId = snapshot.value.roomId;
    commandPending.value = true;
    try {
      requireBackend();
      const result = await api.roomCommand(roomId, { type, payload, expectedVersion: snapshot.value.version, requestId: crypto.randomUUID() });
      if (type !== "room.leave" && type !== "room.end") await loadRoom(roomId);
      return result.event;
    } catch (reason) {
      if (reason instanceof ApiError && reason.code === "version_conflict") {
        await loadRoom(roomId).catch(() => undefined);
        throw new Error("牌桌状态刚刚发生变化，已为你恢复最新状态，请重新操作");
      }
      throw reason;
    } finally {
      commandPending.value = false;
    }
  }

  function connectRoomEvents(roomId: string) {
    if (!apiMode || !backendOnline.value) return;
    shouldReconnect = true;
    eventRoomId = roomId;
    if (eventSocket && (eventSocket.readyState === WebSocket.OPEN || eventSocket.readyState === WebSocket.CONNECTING)) return;
    openRoomEvents();
  }

  function openRoomEvents() {
    if (!shouldReconnect || !eventRoomId) return;
    window.clearTimeout(reconnectTimer);
    connectionState.value = reconnectAttempt === 0 ? "connecting" : "reconnecting";
    const roomId = eventRoomId;
    const socket = new WebSocket(api.webSocketUrl(roomId));
    eventSocket = socket;
    const refreshSnapshot = () => {
      void loadRoom(roomId).catch((reason) => {
        if (reason instanceof ApiError && reason.code === "player_not_seated") {
          shouldReconnect = false;
          connectionState.value = "offline";
          connectionError.value = "你已离开或被移出这个房间";
          socket.close();
          return;
        }
        connectionError.value = reason instanceof Error ? reason.message : "无法恢复牌桌快照";
      });
    };
    socket.addEventListener("open", () => {
      if (eventSocket !== socket) return;
      backendOnline.value = true;
      connectionState.value = "connected";
      connectionError.value = "";
      reconnectAttempt = 0;
      refreshSnapshot();
    });
    socket.addEventListener("message", (message) => {
      try {
        const event = JSON.parse(String(message.data)) as { type: string; payload: unknown };
        if (event.type === "table.snapshot") acceptSnapshot(event.payload as TableSnapshot);
        else refreshSnapshot();
      } catch {
        connectionError.value = "收到无法识别的实时消息，正在恢复权威状态";
        refreshSnapshot();
      }
    });
    socket.addEventListener("close", (event) => {
      if (eventSocket !== socket) return;
      eventSocket = null;
      if (isSessionRevokedClose(event.code)) {
        shouldReconnect = false;
        connectionState.value = "offline";
        connectionError.value = "登录状态已失效，请重新登录";
        clearAccount();
        return;
      }
      if (!shouldReconnect) {
        connectionState.value = "offline";
        return;
      }
      connectionState.value = "reconnecting";
      connectionError.value = "实时连接中断，正在恢复牌桌";
      const delay = reconnectDelay(reconnectAttempt++);
      reconnectTimer = window.setTimeout(openRoomEvents, delay);
    });
    socket.addEventListener("error", () => {
      connectionError.value = "实时连接暂时不可用";
    });
  }

  function disconnectRoomEvents() {
    shouldReconnect = false;
    eventRoomId = "";
    reconnectAttempt = 0;
    window.clearTimeout(reconnectTimer);
    const socket = eventSocket;
    eventSocket = null;
    socket?.close();
    connectionState.value = "offline";
    voice.disconnect();
  }

  async function addAccountPoints(amount: number) {
    if (!Number.isInteger(amount) || amount < 1 || amount > 1_000_000_000) {
      throw new Error("请输入 1 到 1,000,000,000 之间的正整数");
    }
    const waitMs = 5000 - (Date.now() - lastScoreAdditionAt.value);
    if (waitMs > 0) throw new Error(`请等待 ${Math.ceil(waitMs / 1000)} 秒后再增加积分`);

    requireBackend();
    const result = await api.addScore({ amount, roomId: snapshot.value.roomId || undefined, requestId: crypto.randomUUID() });
    accountPoints.value = result.balance;
    ledger.value.unshift(result.entry);

    const me = localPlayer.value;
    if (me) me.accountPoints = result.balance;
    lastScoreAdditionAt.value = Date.now();
  }

  function updateRoomConfig(config: RoomConfig) {
    roomConfig.value = structuredClone(config);
    snapshot.value.allowedChipDenominations = [...config.chipDenominations];
    snapshot.value.roomName = config.name;
  }

  async function toggleMicrophone() {
    if (voiceBusy.value) return;
    voiceError.value = "";
    if (localPlayer.value?.isMuted) {
      voiceError.value = "房主已将你的麦克风静音";
      return;
    }
    if (!roomConfig.value.voiceEnabled) {
      voiceError.value = "这个房间没有开启语音";
      return;
    }
    if (!currentUser.value) {
      voiceError.value = "请先登录账号再开启语音";
      return;
    }
    voiceBusy.value = true;
    try {
      if (microphoneEnabled.value) {
        await voice.disableMicrophone();
        microphoneEnabled.value = false;
        return;
      }
      requireBackend();
      microphoneEnabled.value = await voice.enableMicrophone(snapshot.value.roomId, currentUser.value.id);
      if (microphoneEnabled.value) await refreshMicrophones();
    } finally {
      voiceBusy.value = false;
    }
  }

  async function refreshMicrophones() {
    try {
      const devices = await voice.microphones(false);
      microphones.value = devices.map((device, index) => ({ deviceId: device.deviceId, label: device.label || `麦克风 ${index + 1}` }));
      if (!microphones.value.some((device) => device.deviceId === selectedMicrophoneId.value)) {
        selectedMicrophoneId.value = microphones.value[0]?.deviceId ?? "";
      }
    } catch (reason) {
      voiceError.value = reason instanceof Error ? reason.message : "无法读取麦克风设备";
    }
  }

  async function selectMicrophone(deviceId: string) {
    if (!deviceId || deviceId === selectedMicrophoneId.value) return;
    voiceError.value = "";
    try {
      const switched = await voice.switchMicrophone(deviceId);
      if (!switched) throw new Error("请先开启麦克风再切换设备");
      selectedMicrophoneId.value = deviceId;
    } catch (reason) {
      voiceError.value = reason instanceof Error ? reason.message : "切换麦克风失败";
    }
  }

  async function registerAccount(phone: string, password: string, nickname: string) {
    requireBackend();
    const result = await api.register(phone, password, nickname);
    currentUser.value = result.user;
    accountPoints.value = result.balance;
    activeRoomId.value = "";
    ledger.value = [];
    await refreshAccount();
  }

  async function loginAccount(phone: string, password: string) {
    requireBackend();
    const result = await api.login(phone, password);
    currentUser.value = result.user;
    accountPoints.value = result.balance;
    await refreshAccount();
  }

  async function updateNickname(nickname: string) {
    requireBackend();
    const result = await api.updateMe(nickname);
    currentUser.value = result.user;
    accountPoints.value = result.balance;
    activeRoomId.value = result.activeRoomId ?? "";
  }

  async function logoutAccount() {
    if (apiMode && backendOnline.value) await api.logout();
    disconnectRoomEvents();
    clearAccount();
  }

  function clearAccount() {
    currentUser.value = null;
    accountPoints.value = null;
    activeRoomId.value = "";
    ledger.value = [];
  }

  async function raise(chips: ChipDenomination[]) {
    await sendCommand("action.raise", { chips });
  }

  async function call() {
    await sendCommand(snapshot.value.canCheck ? "action.check" : "action.call");
  }

  async function fold() {
    await sendCommand("action.fold");
  }

  async function allIn() {
    await sendCommand("action.all_in");
  }

  return {
    accountPoints, currentUser, activeRoomId, ledger, roomConfig, snapshot, messages, microphoneEnabled, voiceConnected, voiceTransport, voiceError, activeSpeakerId, microphones, selectedMicrophoneId, voiceBusy,
    backendOnline, connectionState, connectionError, roomLoading, commandPending,
    localPlayer, activePlayers, probeBackend, refreshAccount, acceptSnapshot, createRoom, loadRoom, joinRoom, sendCommand, connectRoomEvents, disconnectRoomEvents,
    addAccountPoints, updateRoomConfig, toggleMicrophone, refreshMicrophones, selectMicrophone, registerAccount, loginAccount, updateNickname, logoutAccount, raise, call, fold, allIn,
  };
});
