import type { ChipDenomination, RoomConfig, ScoreLedgerEntry, TableSnapshot } from "@royal-flush/contracts";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { demoLedger, demoMessages, demoRoomConfig, demoSnapshot } from "@/data/demo";
import { api } from "@/lib/api";

type Message = { id: string; type: string; text: string; at: string };
const useRemoteApi = import.meta.env.VITE_USE_API === "true";

export const useGameStore = defineStore("game", () => {
  const accountPoints = ref(1860);
  const ledger = ref<ScoreLedgerEntry[]>(structuredClone(demoLedger));
  const roomConfig = ref<RoomConfig>(structuredClone(demoRoomConfig));
  const snapshot = ref<TableSnapshot>(structuredClone(demoSnapshot));
  const messages = ref<Message[]>(structuredClone(demoMessages));
  const microphoneEnabled = ref(true);
  const voiceConnected = ref(true);
  const backendOnline = ref(false);
  const lastScoreAdditionAt = ref(0);

  const localPlayer = computed(() => snapshot.value.players.find((player) => player.isLocal));
  const activePlayers = computed(() => snapshot.value.players.filter((player) => player.status !== "away").length);

  async function probeBackend() {
    if (!useRemoteApi) return;
    try {
      await api.health();
      backendOnline.value = true;
    } catch {
      backendOnline.value = false;
    }
  }

  async function addAccountPoints(amount: number) {
    if (!Number.isInteger(amount) || amount < 1 || amount > 1_000_000_000) {
      throw new Error("请输入 1 到 1,000,000,000 之间的正整数");
    }
    const waitMs = 5000 - (Date.now() - lastScoreAdditionAt.value);
    if (waitMs > 0) throw new Error(`请等待 ${Math.ceil(waitMs / 1000)} 秒后再增加积分`);

    if (useRemoteApi && backendOnline.value) {
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

  function toggleMicrophone() {
    microphoneEnabled.value = !microphoneEnabled.value;
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

  function raise(chips: ChipDenomination[]) {
    const raiseBy = chips.reduce<number>((sum, chip) => sum + chip, 0);
    commitAction(`你加注 ${raiseBy}，本轮总投入 ${snapshot.value.toCall + raiseBy}`, snapshot.value.toCall + raiseBy);
  }

  function call() {
    commitAction(snapshot.value.canCheck ? "你选择过牌" : `你跟注 ${snapshot.value.toCall}`, snapshot.value.canCheck ? 0 : snapshot.value.toCall);
  }

  function fold() {
    const me = localPlayer.value;
    if (me) me.status = "folded";
    commitAction("你选择弃牌", 0);
  }

  function allIn() {
    const me = localPlayer.value;
    if (!me) return;
    const amount = me.tablePoints;
    commitAction(`你全下 ${amount}`, amount);
    me.status = "all-in";
  }

  return {
    accountPoints, ledger, roomConfig, snapshot, messages, microphoneEnabled, voiceConnected, backendOnline,
    localPlayer, activePlayers, probeBackend, addAccountPoints, updateRoomConfig, toggleMicrophone, raise, call, fold, allIn,
  };
});

