<script setup lang="ts">
import type { ChipDenomination } from "@royal-flush/contracts";
import { ArrowLeft, Clock3, Copy, History, Mic, MicOff, PanelRightClose, Play, Radio, RefreshCw, Settings, Signal, Users, Wifi, WifiOff } from "@lucide/vue";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import BrandMark from "@/components/BrandMark.vue";
import ChipComposer from "@/components/ChipComposer.vue";
import PlayerSeat from "@/components/PlayerSeat.vue";
import PlayingCard from "@/components/PlayingCard.vue";
import QuickMessagePanel from "@/components/QuickMessagePanel.vue";
import ReportPanel from "@/components/ReportPanel.vue";
import RoomManagementPanel from "@/components/RoomManagementPanel.vue";
import ScoreAddPanel from "@/components/ScoreAddPanel.vue";
import SystemBroadcast from "@/components/SystemBroadcast.vue";
import ThemeSwitcher from "@/components/ThemeSwitcher.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { ApiError, apiMode } from "@/lib/api";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const route = useRoute();
const router = useRouter();
const sidePanel = ref<"history" | "settings" | null>(null);
const tableError = ref("");
const roomCodeCopied = ref(false);
const now = ref(Date.now());
let timer = 0;

const broadcast = computed(() => store.connectionError ? { text: store.connectionError, at: new Date().toLocaleTimeString("zh-CN", { hour12: false }) } : store.messages[0] ?? { text: "牌局已连接", at: "--:--:--" });
const remainingSeconds = computed(() => Math.max(0, Math.ceil((Date.parse(store.snapshot.actionDeadline) - now.value) / 1000)));
const acting = computed(() => Boolean(store.localPlayer?.isCurrentActor));
const actionProgress = computed(() => {
  const duration = Math.max(1, store.roomConfig.actionSeconds);
  return Math.max(0, Math.min(1, remainingSeconds.value / duration));
});
const speakingPlayer = computed(() => store.voiceConnected ? store.snapshot.players.find((player) => player.isSpeaking) : undefined);
const isOwner = computed(() => Boolean(store.localPlayer && store.snapshot.ownerId === store.localPlayer.id));
const handSettled = computed(() => store.snapshot.street === "settled");
const readyForNextHand = computed(() => store.snapshot.players.filter((player) => player.isReady && player.status !== "away" && player.status !== "disconnected" && player.tablePoints > 0).length);
const canStartNextHand = computed(() => handSettled.value && isOwner.value && readyForNextHand.value >= 2 && !store.commandPending && store.connectionState === "connected");
const canAct = computed(() => acting.value && !store.commandPending && store.connectionState === "connected");
const connectionLabel = computed(() => ({ offline: "实时连接离线", connecting: "正在连接牌桌", connected: "连接稳定", reconnecting: "正在恢复牌桌" })[store.connectionState]);
const voiceConnectionLabel = computed(() => store.voiceConnected ? (store.voiceTransport === "livekit" ? "云端语音已连接" : "直连语音已连接") : "语音未连接");

watch(() => [store.snapshot.roomId, store.snapshot.street] as const, ([roomId, street]) => {
  if (street === "waiting" && roomId === String(route.params.id) && route.name === "table") {
    void router.replace(`/rooms/${roomId}/waiting`);
  }
});

function playerForSeat(seat: number) {
  return store.snapshot.players.find((player) => player.seat === seat);
}

async function copyRoomCode() {
	tableError.value = "";
	try {
		await navigator.clipboard.writeText(store.snapshot.roomCode);
		roomCodeCopied.value = true;
		window.setTimeout(() => (roomCodeCopied.value = false), 1800);
	} catch {
		roomCodeCopied.value = false;
		tableError.value = "无法复制房间码，请手动记录";
	}
}

async function playAction(action: () => Promise<void>) {
	if (store.commandPending) return;
  tableError.value = "";
  try {
    await action();
  } catch (reason) {
    tableError.value = reason instanceof Error ? reason.message : "牌局操作失败";
  }
}

function raise(chips: ChipDenomination[]) {
  return playAction(() => store.raise(chips));
}

async function retryConnection() {
  tableError.value = "";
  await store.probeBackend();
  if (!store.backendOnline) {
    tableError.value = "后端服务暂时不可用，请稍后重试";
    return;
  }
  try {
    const snapshot = await store.loadRoom(String(route.params.id));
    store.connectRoomEvents(snapshot.roomId);
  } catch (reason) {
    if (await redirectAfterUnavailableRoom(reason)) return;
    tableError.value = reason instanceof Error ? reason.message : "无法恢复牌桌";
  }
}

async function leaveTable() {
	if (store.commandPending) return;
  if (apiMode && !window.confirm("离开牌桌并结算当前座位？")) return;
  tableError.value = "";
  try {
    if (apiMode && store.backendOnline) await store.sendCommand("room.leave");
    store.disconnectRoomEvents();
    await router.push("/");
  } catch (reason) {
    tableError.value = reason instanceof Error ? reason.message : "离桌失败";
  }
}

async function refill() {
  await playAction(() => store.sendCommand("room.refill").then(() => undefined));
}

async function startNextHand() {
  if (!canStartNextHand.value) return;
  await playAction(() => store.sendCommand("game.start").then(() => undefined));
}

function selectMicrophone(event: Event) {
  void store.selectMicrophone((event.target as HTMLSelectElement).value);
}

async function redirectAfterUnavailableRoom(reason: unknown) {
  const recoverable = reason instanceof ApiError && (
    reason.status === 401 ||
    reason.status === 404 ||
    reason.status === 410 ||
    ["player_not_seated", "room_not_found", "room_closed"].includes(reason.code)
  );
  if (!recoverable) return false;

  store.disconnectRoomEvents();
  try {
    await store.refreshAccount();
  } catch {
    return false;
  }
  if (!store.currentUser) {
    await router.replace({ name: "account", query: { redirect: route.fullPath } });
  } else {
    await router.replace(store.activeRoomRoute());
  }
  return true;
}

onMounted(async () => {
  timer = window.setInterval(() => (now.value = Date.now()), 250);
  await store.probeBackend();
  if (store.backendOnline) {
    try {
      const snapshot = await store.loadRoom(String(route.params.id));
      store.connectRoomEvents(snapshot.roomId);
    } catch (reason) {
      if (await redirectAfterUnavailableRoom(reason)) return;
      tableError.value = reason instanceof Error ? reason.message : "无法读取牌桌";
    }
  }
});
onBeforeUnmount(() => {
  window.clearInterval(timer);
  store.disconnectRoomEvents();
});
</script>

<template>
  <main class="table-app">
    <header class="table-topbar">
      <div class="table-room-brand"><BrandMark compact /><div><strong>{{ store.snapshot.roomName }}</strong><button type="button" :title="roomCodeCopied ? '房间码已复制' : '复制房间码'" :aria-label="roomCodeCopied ? '房间码已复制' : '复制房间码'" @click="copyRoomCode">{{ roomCodeCopied ? "已复制" : store.snapshot.roomCode }}<Copy /></button></div></div>
      <div class="station-voice"><VoiceMeter :active="Boolean(speakingPlayer)" /><div><strong>{{ voiceConnectionLabel }}</strong><span>{{ speakingPlayer ? `${speakingPlayer.name}正在说话` : "当前无人说话" }} · {{ store.activePlayers }} 人在线</span></div></div>
      <div class="table-tools"><span class="network-status" :class="{ unstable: store.connectionState !== 'connected' }"><Wifi v-if="store.connectionState === 'connected'" /><WifiOff v-else />{{ connectionLabel }}</span><ThemeSwitcher compact /><button class="icon-button" type="button" :class="{ active: store.microphoneEnabled }" :disabled="store.voiceBusy" :title="store.microphoneEnabled ? '关闭麦克风' : '开启麦克风'" :aria-label="store.microphoneEnabled ? '关闭麦克风' : '开启麦克风'" @click="store.toggleMicrophone"><Mic v-if="store.microphoneEnabled" /><MicOff v-else /></button><button class="icon-button" type="button" title="牌局记录" aria-label="牌局记录" @click="sidePanel = sidePanel === 'history' ? null : 'history'"><History /></button><button class="icon-button" type="button" title="牌桌设置" aria-label="牌桌设置" @click="sidePanel = sidePanel === 'settings' ? null : 'settings'"><Settings /></button><button class="icon-button" type="button" title="离开牌桌" aria-label="离开牌桌" :disabled="store.commandPending" @click="leaveTable"><ArrowLeft /></button></div>
    </header>

    <SystemBroadcast :text="broadcast.text" :at="broadcast.at" />

    <section class="table-stage" aria-label="德州扑克牌桌">
      <div class="table-telemetry"><span :class="{ unstable: store.connectionState !== 'connected' }"><Signal />{{ connectionLabel }}</span><span>手牌 <strong>{{ String(store.snapshot.handNumber).padStart(3, "0") }}</strong></span><span>盲注 <strong>{{ store.roomConfig.blindPreset }}</strong></span></div>
      <div v-if="tableError || (apiMode && store.connectionState !== 'connected')" class="table-state-banner" role="status"><WifiOff /><span>{{ tableError || connectionLabel }}</span><button type="button" :disabled="store.roomLoading" @click="retryConnection"><RefreshCw />{{ store.roomLoading ? "恢复中" : "重新连接" }}</button></div>
      <div v-else-if="handSettled" class="table-state-banner next-hand" role="status"><Play /><span>{{ isOwner ? (readyForNextHand >= 2 ? "本手已结算，可以开始下一手" : "至少需要两名在线且有牌桌分的玩家") : "本手已结算，等待房主开始下一手" }}</span><button v-if="isOwner" type="button" :disabled="!canStartNextHand" @click="startNextHand"><Play />{{ store.commandPending ? "开牌中" : "开始下一手" }}</button></div>
      <div class="poker-table-shell">
        <div class="poker-table">
          <div class="table-center">
            <div class="pot-readout"><span>当前底池</span><strong>{{ store.snapshot.pot.toLocaleString("zh-CN") }}</strong></div>
            <div class="community-cards" aria-label="公共牌"><PlayingCard v-for="(card, index) in store.snapshot.board" :key="`${card.rank}-${card.suit}-${index}`" :card="card" /></div>
            <span class="street-label">{{ { preflop: '翻牌前', flop: '翻牌', turn: '转牌', river: '河牌', showdown: '摊牌', settled: '已结算', waiting: '等待开局' }[store.snapshot.street] }}</span>
          </div>
        </div>
        <PlayerSeat v-for="seat in 8" :key="seat - 1" :seat="seat - 1" :player="playerForSeat(seat - 1)" :turn-progress="actionProgress" :voice-connected="store.voiceConnected" />
        <div class="local-hole-cards" aria-label="你的底牌"><PlayingCard v-for="card in store.snapshot.holeCards" :key="`${card.rank}-${card.suit}`" :card="card" compact /></div>
      </div>
    </section>

    <footer class="table-dock">
      <section class="dock-voice">
        <header class="dock-heading"><span>桌内语音</span><strong>{{ store.activePlayers }} / {{ store.roomConfig.maxPlayers }}</strong></header>
        <button class="active-speaker" type="button" :disabled="!store.roomConfig.voiceEnabled" @click="store.toggleMicrophone"><span class="speaker-avatar">{{ speakingPlayer?.initials ?? '—' }}</span><span><strong>{{ store.roomConfig.voiceEnabled ? (speakingPlayer?.name ?? '当前无人说话') : '房间未开启语音' }}</strong><small>{{ store.voiceConnected ? (speakingPlayer ? '正在说话 · 信号良好' : voiceConnectionLabel) : '语音不可用时不影响牌局' }}</small></span><VoiceMeter :active="Boolean(speakingPlayer)" /></button>
      </section>
      <ChipComposer class="dock-actions" :denominations="store.snapshot.allowedChipDenominations" :to-call="store.snapshot.toCall" :minimum-raise-by="store.snapshot.minimumRaiseBy" :maximum-raise-by="store.snapshot.maximumRaiseBy" :can-check="store.snapshot.canCheck" :can-raise="store.snapshot.canRaise" :can-all-in="store.snapshot.canAllIn" :disabled="!canAct" @raise="raise" @call="playAction(store.call)" @fold="playAction(store.fold)" @all-in="playAction(store.allIn)" />
      <div class="action-timer" :class="{ urgent: remainingSeconds <= 8, inactive: !acting }"><Clock3 /><span>{{ acting ? "你的行动" : "等待行动" }}</span><strong>{{ acting ? `${remainingSeconds} 秒` : "—" }}</strong><i :style="{ '--progress': acting ? `${Math.max(0, remainingSeconds / store.roomConfig.actionSeconds) * 100}%` : '0%' }" /></div>
    </footer>

    <Transition name="panel">
      <aside v-if="sidePanel" class="table-side-panel" :aria-label="sidePanel === 'history' ? '牌局记录' : '牌桌设置'">
        <header><div><Radio v-if="sidePanel === 'history'" /><Settings v-else /><h2>{{ sidePanel === 'history' ? "牌局记录" : "牌桌设置" }}</h2></div><button class="icon-button" type="button" title="关闭面板" aria-label="关闭面板" @click="sidePanel = null"><PanelRightClose /></button></header>
        <template v-if="sidePanel === 'history'">
          <ol class="event-log"><li v-for="message in store.messages" :key="message.id"><time>{{ message.at }}</time><span :class="message.type" /><p>{{ message.text }}</p></li><li v-if="!store.messages.length" class="event-log-empty"><p>暂无牌局记录</p></li></ol>
        </template>
        <template v-else>
          <ScoreAddPanel />
          <section class="side-rules"><header><h3>房间规则</h3><Users /></header><dl><div><dt>人数</dt><dd>{{ store.roomConfig.maxPlayers }} 人</dd></div><div><dt>盲注</dt><dd>{{ store.roomConfig.blindPreset }}</dd></div><div><dt>行动时间</dt><dd>{{ store.roomConfig.actionSeconds }} 秒</dd></div><div><dt>筹码</dt><dd>{{ store.snapshot.allowedChipDenominations.join(" / ") }}</dd></div></dl></section>
          <section class="side-voice"><header><h3>麦克风</h3><Mic /></header><button class="toggle-row" type="button" :disabled="store.voiceBusy || store.localPlayer?.isMuted || !store.roomConfig.voiceEnabled" @click="store.toggleMicrophone"><span><strong>{{ !store.roomConfig.voiceEnabled ? "房间未开启" : store.localPlayer?.isMuted ? "已被房主禁言" : store.voiceBusy ? "正在连接" : store.microphoneEnabled ? "已开启" : "已关闭" }}</strong><small>{{ store.voiceError || (store.voiceTransport === "webrtc" ? "浏览器直连 · 不录制" : store.voiceTransport === "livekit" ? "LiveKit 云端语音 · 不录制" : "语音不录制、不保存、不转写") }}</small></span><span class="switch" :class="{ checked: store.microphoneEnabled }" /></button><select v-if="store.microphones.length > 1" class="device-select" aria-label="麦克风设备" :value="store.selectedMicrophoneId" @change="selectMicrophone"><option v-for="device in store.microphones" :key="device.deviceId" :value="device.deviceId">{{ device.label }}</option></select></section>
          <button v-if="store.localPlayer?.tablePoints === 0" class="tool-button wide refill-button" type="button" :disabled="store.commandPending" @click="refill">重新获得 1,000 牌桌分</button>
          <QuickMessagePanel />
          <ReportPanel />
          <RoomManagementPanel v-if="isOwner" @room-ended="router.push('/')" />
        </template>
      </aside>
    </Transition>
  </main>
</template>
