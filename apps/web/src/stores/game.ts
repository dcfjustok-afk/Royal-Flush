import type { ChipDenomination, RoomConfig, ScoreLedgerEntry, TableSnapshot } from "@royal-flush/contracts";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { demoLedger, demoMessages, demoRoomConfig, demoSnapshot } from "@/data/demo";
import { api, apiMode } from "@/lib/api";
import { VoiceController } from "@/lib/voice";

type Message = { id: string; type: string; text: string; at: string };

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
  const backendOnline = ref(false);
  const lastScoreAdditionAt = ref(0);
  let eventSocket: WebSocket | null = null;
  const voice = new VoiceController({
    connected: (connected) => (voiceConnected.value = connected),
    activeSpeaker: (identity) => {
      activeSpeakerId.value = identity;
      for (const player of snapshot.value.players) player.isSpeaking = player.id === identity;
    },
    error: (message) => (voiceError.value = message),
  });

  const localPlayer = computed(() => snapshot.value.players.find((player) => player.isLocal));
  const activePlayers = computed(() => snapshot.value.players.filter((player) => player.status !== "away").length);

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
    const me = next.players.find((player) => player.isLocal);
    if (me) accountPoints.value = me.accountPoints;
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
    const next = await api.roomSnapshot(roomId);
    acceptSnapshot(next);
    return next;
  }

  async function joinRoom(idOrCode: string, seat: number) {
    const next = await api.joinRoom(idOrCode, seat);
    acceptSnapshot(next);
    backendOnline.value = true;
    return next;
  }

  async function sendCommand(type: string, payload: Record<string, unknown> = {}) {
    const roomId = snapshot.value.roomId;
    const result = await api.roomCommand(roomId, { type, payload, expectedVersion: snapshot.value.version, requestId: crypto.randomUUID() });
    await loadRoom(roomId);
    return result.event;
  }

  function connectRoomEvents(roomId: string) {
    if (!apiMode || !backendOnline.value || eventSocket?.readyState === WebSocket.OPEN) return;
    eventSocket?.close();
    const socket = new WebSocket(api.webSocketUrl(roomId));
    eventSocket = socket;
    socket.addEventListener("open", () => (backendOnline.value = true));
    socket.addEventListener("message", (message) => {
      const event = JSON.parse(String(message.data)) as { type: string; payload: unknown };
      if (event.type === "table.snapshot") acceptSnapshot(event.payload as TableSnapshot);
      else void loadRoom(roomId);
    });
    socket.addEventListener("close", () => {
      if (eventSocket === socket) eventSocket = null;
    });
  }

  function disconnectRoomEvents() {
    eventSocket?.close();
    eventSocket = null;
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
    voiceError.value = "";
    if (microphoneEnabled.value) {
      await voice.disableMicrophone();
      microphoneEnabled.value = false;
      return;
    }
    if (!apiMode || !backendOnline.value) {
      microphoneEnabled.value = true;
      voiceConnected.value = true;
      return;
    }
    microphoneEnabled.value = await voice.enableMicrophone(snapshot.value.roomId);
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
    accountPoints, ledger, roomConfig, snapshot, messages, microphoneEnabled, voiceConnected, voiceError, activeSpeakerId, backendOnline,
    localPlayer, activePlayers, probeBackend, refreshAccount, acceptSnapshot, createRoom, loadRoom, joinRoom, sendCommand, connectRoomEvents, disconnectRoomEvents,
    addAccountPoints, updateRoomConfig, toggleMicrophone, raise, call, fold, allIn,
  };
});
