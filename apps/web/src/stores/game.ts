import type { ChipDenomination, RoomConfig, RoomEvent, ScoreLedgerEntry, TableSnapshot } from "@royal-flush/contracts";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { demoLedger, demoMessages, demoRoomConfig, demoSnapshot } from "@/data/demo";
import { api, ApiError, apiMode } from "@/lib/api";
import { VoiceController } from "@/lib/voice";

type Message = { id: string; type: string; text: string; at: string };
export type RealtimeConnectionState = "offline" | "connecting" | "connected" | "reconnecting";

export function reconnectDelay(attempt: number) {
  return Math.min(250 * 2 ** Math.max(0, attempt), 2000);
}

export const useGameStore = defineStore("game", () => {
  const accountPoints = ref(1860);
  const ledger = ref<ScoreLedgerEntry[]>(structuredClone(demoLedger));
  const roomConfig = ref<RoomConfig>(structuredClone(demoRoomConfig));
  const snapshot = ref<TableSnapshot>(structuredClone(demoSnapshot));
  const messages = ref<Message[]>(structuredClone(demoMessages));
  const microphoneEnabled = ref(false);
  const voiceConnected = ref(false);
  const voiceError = ref("");
  const activeSpeakerId = ref<string | null>(null);
  const microphones = ref<Array<{ deviceId: string; label: string }>>([]);
  const selectedMicrophoneId = ref("");
  const voiceBusy = ref(false);
  const backendOnline = ref(false);
  const connectionState = ref<RealtimeConnectionState>(apiMode ? "offline" : "connected");
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
    const [me, scoreLedger] = await Promise.all([api.me(), api.scoreLedger()]);
    accountPoints.value = me.balance;
    ledger.value = scoreLedger.entries;
    if (me.activeRoomId) await loadRoom(me.activeRoomId).catch(() => undefined);
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
      if (me.isMuted && microphoneEnabled.value) {
        void voice.disableMicrophone();
        microphoneEnabled.value = false;
        voiceError.value = "房主已将你的麦克风静音";
      }
    }
  }

  async function createRoom(config: RoomConfig) {
    updateRoomConfig(config);
    if (!apiMode || !backendOnline.value) return { id: "room-new", code: "RF-NEW" };
    const created = await api.createRoom(config);
    acceptSnapshot(created.snapshot);
    return { id: created.id, code: created.code };
  }

  async function loadRoom(roomId: string) {
    if (!apiMode || !backendOnline.value) return snapshot.value;
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

  function applyDemoCommand(type: string, payload: Record<string, unknown>) {
    const userId = String(payload.userId ?? "");
    const target = snapshot.value.players.find((player) => player.id === userId);
    switch (type) {
      case "room.ready":
        if (localPlayer.value) localPlayer.value.isReady = Boolean(payload.ready);
        break;
      case "room.quick_message":
        messages.value.unshift({ id: crypto.randomUUID(), type: "quick", text: `${localPlayer.value?.name ?? "你"}：${String(payload.message)}`, at: new Date().toLocaleTimeString("zh-CN", { hour12: false }) });
        break;
      case "voice.mute":
        if (target) target.isMuted = Boolean(payload.muted);
        break;
      case "room.remove_player":
        snapshot.value.players = snapshot.value.players.filter((player) => player.id !== userId);
        break;
      case "room.rotate_invite":
        snapshot.value.roomCode = `RF-${Math.floor(1000 + Math.random() * 9000)}`;
        break;
      case "room.transfer_owner":
        snapshot.value.ownerId = userId;
        break;
      case "room.refill":
        if (localPlayer.value?.tablePoints === 0) {
          localPlayer.value.tablePoints = 1000;
          localPlayer.value.status = "active";
        }
        break;
      case "room.leave":
        snapshot.value.players = snapshot.value.players.filter((player) => !player.isLocal);
        break;
      case "room.end":
        snapshot.value.ended = true;
        break;
    }
    snapshot.value.version += 1;
    return {
      type,
      requestId: crypto.randomUUID(),
      roomId: snapshot.value.roomId,
      version: snapshot.value.version,
      sentAt: new Date().toISOString(),
      payload,
    } satisfies RoomEvent;
  }

  async function sendCommand(type: string, payload: Record<string, unknown> = {}) {
    if (commandPending.value) throw new Error("上一项操作仍在处理中");
    const roomId = snapshot.value.roomId;
    commandPending.value = true;
    try {
      if (!apiMode || !backendOnline.value) return applyDemoCommand(type, payload);
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
    socket.addEventListener("close", () => {
      if (eventSocket !== socket) return;
      eventSocket = null;
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

    if (apiMode && backendOnline.value) {
      const result = await api.addScore({ amount, roomId: snapshot.value.roomId, requestId: crypto.randomUUID() });
      accountPoints.value = result.balance;
      ledger.value.unshift(result.entry);
    } else {
      accountPoints.value += amount;
      ledger.value.unshift({
        id: crypto.randomUUID(), type: "self_add", amount, balance: accountPoints.value,
        roomId: snapshot.value.roomId, note: "自行增加积分", createdAt: new Date().toISOString(),
      });
    }

    const me = localPlayer.value;
    if (me) me.accountPoints = accountPoints.value;
    messages.value.unshift({ id: crypto.randomUUID(), type: "score", text: `你自行增加了 ${amount.toLocaleString("zh-CN")} 积分，当前局外积分为 ${accountPoints.value.toLocaleString("zh-CN")}`, at: new Date().toLocaleTimeString("zh-CN", { hour12: false }) });
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
    voiceBusy.value = true;
    try {
      if (microphoneEnabled.value) {
        await voice.disableMicrophone();
        microphoneEnabled.value = false;
        return;
      }
      if (!apiMode || !backendOnline.value) {
        microphoneEnabled.value = true;
        voiceConnected.value = true;
        microphones.value = [{ deviceId: "default", label: "系统默认麦克风" }];
        selectedMicrophoneId.value = "default";
        return;
      }
      microphoneEnabled.value = await voice.enableMicrophone(snapshot.value.roomId);
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

  function commitAction(label: string, cost: number) {
    const me = localPlayer.value;
    if (!me || !me.isCurrentActor) return;
    me.tablePoints = Math.max(0, me.tablePoints - cost);
    me.streetCommitted += cost;
    me.isCurrentActor = false;
    const next = snapshot.value.players.find((player) => !player.isLocal && player.status === "active");
    if (next) next.isCurrentActor = true;
    snapshot.value.version += 1;
    snapshot.value.pot += cost;
    messages.value.unshift({ id: crypto.randomUUID(), type: "action", text: label, at: new Date().toLocaleTimeString("zh-CN", { hour12: false }) });
  }

  async function raise(chips: ChipDenomination[]) {
    if (apiMode && backendOnline.value) {
      await sendCommand("action.raise", { chips });
      return;
    }
    const raiseBy = chips.reduce<number>((sum, chip) => sum + chip, 0);
    commitAction(`你加注 ${raiseBy}，本轮总投入 ${snapshot.value.toCall + raiseBy}`, snapshot.value.toCall + raiseBy);
  }

  async function call() {
    if (apiMode && backendOnline.value) {
      await sendCommand(snapshot.value.canCheck ? "action.check" : "action.call");
      return;
    }
    commitAction(snapshot.value.canCheck ? "你选择过牌" : `你跟注 ${snapshot.value.toCall}`, snapshot.value.canCheck ? 0 : snapshot.value.toCall);
  }

  async function fold() {
    if (apiMode && backendOnline.value) {
      await sendCommand("action.fold");
      return;
    }
    const me = localPlayer.value;
    if (me) me.status = "folded";
    commitAction("你选择弃牌", 0);
  }

  async function allIn() {
    if (apiMode && backendOnline.value) {
      await sendCommand("action.all_in");
      return;
    }
    const me = localPlayer.value;
    if (!me) return;
    const amount = me.tablePoints;
    commitAction(`你全下 ${amount}`, amount);
    me.status = "all-in";
  }

  return {
    accountPoints, ledger, roomConfig, snapshot, messages, microphoneEnabled, voiceConnected, voiceError, activeSpeakerId, microphones, selectedMicrophoneId, voiceBusy,
    backendOnline, connectionState, connectionError, roomLoading, commandPending,
    localPlayer, activePlayers, probeBackend, refreshAccount, acceptSnapshot, createRoom, loadRoom, joinRoom, sendCommand, connectRoomEvents, disconnectRoomEvents,
    addAccountPoints, updateRoomConfig, toggleMicrophone, refreshMicrophones, selectMicrophone, raise, call, fold, allIn,
  };
});
