<script setup lang="ts">
import { ArrowLeft, Clock3, Copy, History, Mic, MicOff, PanelRightClose, Radio, Settings, Signal, Users, Wifi } from "@lucide/vue";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import BrandMark from "@/components/BrandMark.vue";
import ChipComposer from "@/components/ChipComposer.vue";
import PlayerSeat from "@/components/PlayerSeat.vue";
import PlayingCard from "@/components/PlayingCard.vue";
import ScoreAddPanel from "@/components/ScoreAddPanel.vue";
import SystemBroadcast from "@/components/SystemBroadcast.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const route = useRoute();
const sidePanel = ref<"history" | "settings" | null>(null);
const now = ref(Date.now());
let timer = 0;

const broadcast = computed(() => store.messages[0] ?? { text: "牌局已连接", at: "--:--:--" });
const remainingSeconds = computed(() => Math.max(0, Math.ceil((Date.parse(store.snapshot.actionDeadline) - now.value) / 1000)));
const acting = computed(() => Boolean(store.localPlayer?.isCurrentActor));
const speakingPlayer = computed(() => store.snapshot.players.find((player) => player.isSpeaking));

function playerForSeat(seat: number) {
  return store.snapshot.players.find((player) => player.seat === seat);
}

async function copyRoomCode() {
  await navigator.clipboard.writeText(store.snapshot.roomCode).catch(() => undefined);
}

onMounted(async () => {
  timer = window.setInterval(() => (now.value = Date.now()), 250);
  await store.probeBackend();
  if (store.backendOnline) {
    await store.loadRoom(String(route.params.id)).catch(() => undefined);
    store.connectRoomEvents(store.snapshot.roomId);
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
      <div class="table-room-brand"><BrandMark compact /><div><strong>{{ store.snapshot.roomName }}</strong><button type="button" title="复制房间码" @click="copyRoomCode">{{ store.snapshot.roomCode }}<Copy /></button></div></div>
      <div class="station-voice"><VoiceMeter :active="Boolean(speakingPlayer)" /><div><strong>{{ store.voiceConnected ? "桌内语音已连接" : "语音连接中断" }}</strong><span>{{ speakingPlayer ? `${speakingPlayer.name}正在说话` : "当前无人说话" }} · {{ store.activePlayers }} 人在线</span></div></div>
      <div class="table-tools"><span class="network-status"><Wifi />延迟 38ms</span><button class="icon-button" type="button" :class="{ active: store.microphoneEnabled }" :title="store.microphoneEnabled ? '关闭麦克风' : '开启麦克风'" :aria-label="store.microphoneEnabled ? '关闭麦克风' : '开启麦克风'" @click="store.toggleMicrophone"><Mic v-if="store.microphoneEnabled" /><MicOff v-else /></button><button class="icon-button" type="button" title="牌局记录" aria-label="牌局记录" @click="sidePanel = sidePanel === 'history' ? null : 'history'"><History /></button><button class="icon-button" type="button" title="牌桌设置" aria-label="牌桌设置" @click="sidePanel = sidePanel === 'settings' ? null : 'settings'"><Settings /></button><RouterLink class="icon-button" to="/" title="离开牌桌" aria-label="离开牌桌"><ArrowLeft /></RouterLink></div>
    </header>

    <SystemBroadcast :text="broadcast.text" :at="broadcast.at" />

    <section class="table-stage" aria-label="德州扑克牌桌">
      <div class="table-telemetry"><span><Signal />连接稳定</span><span>手牌 <strong>{{ String(store.snapshot.handNumber).padStart(3, "0") }}</strong></span><span>盲注 <strong>{{ store.roomConfig.blindPreset }}</strong></span></div>
      <div class="poker-table-shell">
        <div class="poker-table">
          <div class="table-center">
            <div class="pot-readout"><span>当前底池</span><strong>{{ store.snapshot.pot.toLocaleString("zh-CN") }}</strong></div>
            <div class="community-cards" aria-label="公共牌"><PlayingCard v-for="(card, index) in store.snapshot.board" :key="`${card.rank}-${card.suit}-${index}`" :card="card" /></div>
            <span class="street-label">{{ { preflop: '翻牌前', flop: '翻牌', turn: '转牌', river: '河牌', showdown: '摊牌', settled: '已结算', waiting: '等待开局' }[store.snapshot.street] }}</span>
          </div>
        </div>
        <PlayerSeat v-for="seat in 8" :key="seat - 1" :seat="seat - 1" :player="playerForSeat(seat - 1)" />
        <div class="local-hole-cards" aria-label="你的底牌"><PlayingCard v-for="card in store.snapshot.holeCards" :key="`${card.rank}-${card.suit}`" :card="card" compact /></div>
      </div>
    </section>

    <footer class="table-dock">
      <section class="dock-voice">
        <header class="dock-heading"><span>桌内语音</span><strong>{{ store.activePlayers }} / {{ store.roomConfig.maxPlayers }}</strong></header>
        <button class="active-speaker" type="button" @click="store.toggleMicrophone"><span class="speaker-avatar">{{ speakingPlayer?.initials ?? '—' }}</span><span><strong>{{ speakingPlayer?.name ?? '当前无人说话' }}</strong><small>{{ store.voiceConnected ? (speakingPlayer ? '正在说话 · 信号良好' : '桌内语音已连接') : '语音不可用，不影响牌局' }}</small></span><VoiceMeter :active="Boolean(speakingPlayer)" /></button>
      </section>
      <ChipComposer class="dock-actions" :denominations="store.snapshot.allowedChipDenominations" :to-call="store.snapshot.toCall" :minimum-raise-by="store.snapshot.minimumRaiseBy" :maximum-raise-by="store.snapshot.maximumRaiseBy" :can-check="store.snapshot.canCheck" :can-raise="store.snapshot.canRaise" :can-all-in="store.snapshot.canAllIn" :disabled="!acting" @raise="store.raise" @call="store.call" @fold="store.fold" @all-in="store.allIn" />
      <div class="action-timer" :class="{ urgent: remainingSeconds <= 8, inactive: !acting }"><Clock3 /><span>{{ acting ? "你的行动" : "等待行动" }}</span><strong>{{ acting ? `${remainingSeconds} 秒` : "—" }}</strong><i :style="{ '--progress': acting ? `${Math.max(0, remainingSeconds / store.roomConfig.actionSeconds) * 100}%` : '0%' }" /></div>
    </footer>

    <Transition name="panel">
      <aside v-if="sidePanel" class="table-side-panel" :aria-label="sidePanel === 'history' ? '牌局记录' : '牌桌设置'">
        <header><div><Radio v-if="sidePanel === 'history'" /><Settings v-else /><h2>{{ sidePanel === 'history' ? "牌局记录" : "牌桌设置" }}</h2></div><button class="icon-button" type="button" title="关闭面板" aria-label="关闭面板" @click="sidePanel = null"><PanelRightClose /></button></header>
        <template v-if="sidePanel === 'history'">
          <ol class="event-log"><li v-for="message in store.messages" :key="message.id"><time>{{ message.at }}</time><span :class="message.type" /><p>{{ message.text }}</p></li></ol>
        </template>
        <template v-else>
          <ScoreAddPanel />
          <section class="side-rules"><header><h3>房间规则</h3><Users /></header><dl><div><dt>人数</dt><dd>{{ store.roomConfig.maxPlayers }} 人</dd></div><div><dt>盲注</dt><dd>{{ store.roomConfig.blindPreset }}</dd></div><div><dt>行动时间</dt><dd>{{ store.roomConfig.actionSeconds }} 秒</dd></div><div><dt>筹码</dt><dd>{{ store.snapshot.allowedChipDenominations.join(" / ") }}</dd></div></dl></section>
          <section class="side-voice"><header><h3>麦克风</h3><Mic /></header><button class="toggle-row" type="button" @click="store.toggleMicrophone"><span><strong>{{ store.microphoneEnabled ? "已开启" : "已关闭" }}</strong><small>{{ store.voiceError || "语音不录制、不保存、不转写" }}</small></span><span class="switch" :class="{ checked: store.microphoneEnabled }" /></button></section>
        </template>
      </aside>
    </Transition>
  </main>
</template>
