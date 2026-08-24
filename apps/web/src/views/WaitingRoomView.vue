<script setup lang="ts">
import { Check, Copy, Headphones, Link2, Mic, MicOff, Play, Settings2, UserPlus } from "@lucide/vue";
import { computed, onMounted, ref } from "vue";
import { onBeforeRouteLeave, useRoute, useRouter } from "vue-router";
import AppHeader from "@/components/AppHeader.vue";
import RoomManagementPanel from "@/components/RoomManagementPanel.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { ApiError, apiMode } from "@/lib/api";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const route = useRoute();
const router = useRouter();
const copied = ref(false);
const actionError = ref("");
const inviteUrl = computed(() => `${location.origin}/invite/${store.snapshot.roomCode}`);
const isOwner = computed(() => !store.snapshot.ownerId || store.snapshot.ownerId === store.localPlayer?.id);
const ready = computed(() => Boolean(store.localPlayer?.isReady));
const readyCount = computed(() => store.snapshot.players.filter((player) => player.isReady && player.status !== "disconnected" && player.status !== "away" && player.tablePoints > 0).length);
const seats = computed(() => Array.from({ length: 8 }, (_, seat) => ({
  seat,
  player: store.snapshot.players.find((player) => player.seat === seat),
  available: seat < store.roomConfig.maxPlayers,
})));

async function copyInvite() {
  await navigator.clipboard.writeText(inviteUrl.value).catch(() => undefined);
  copied.value = true;
  window.setTimeout(() => (copied.value = false), 1800);
}

async function toggleReady() {
	if (store.commandPending) return;
	actionError.value = "";
  const next = !ready.value;
  try {
    await store.sendCommand("room.ready", { ready: next });
  } catch (reason) {
    actionError.value = reason instanceof Error ? reason.message : "更新准备状态失败";
  }
}

async function startGame() {
	if (store.commandPending) return;
	actionError.value = "";
  try {
    await store.sendCommand("game.start");
    await router.push(`/rooms/${store.snapshot.roomId}/table`);
  } catch (reason) {
    actionError.value = reason instanceof Error ? reason.message : "无法开始牌局";
  }
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
  await store.probeBackend();
  if (!apiMode || !store.backendOnline) {
    actionError.value = "服务暂时不可用，请稍后重试";
    return;
  }
  try {
    const snapshot = await store.loadRoom(String(route.params.id));
    store.connectRoomEvents(snapshot.roomId);
  } catch (reason) {
    if (await redirectAfterUnavailableRoom(reason)) return;
    actionError.value = reason instanceof Error ? reason.message : "无法恢复房间状态";
  }
});

onBeforeRouteLeave((to) => {
  if (to.name !== "table") store.disconnectRoomEvents();
});

function selectMicrophone(event: Event) {
  void store.selectMicrophone((event.target as HTMLSelectElement).value);
}
</script>

<template>
  <div class="page-shell waiting-page">
    <AppHeader />
    <main class="waiting-content">
      <header class="waiting-heading"><div><span class="live-label"><VoiceMeter active />好友正在加入</span><h1>{{ store.roomConfig.name }}</h1><p>{{ store.snapshot.roomCode }} · {{ isOwner ? "你是房主" : "等待房主开局" }}</p></div><button class="button light" type="button" @click="copyInvite"><Check v-if="copied" /><Copy v-else />{{ copied ? "已复制" : "复制邀请链接" }}</button></header>
      <section class="waiting-table-area">
        <div class="waiting-table">
          <div class="waiting-table-center"><strong>{{ store.snapshot.players.length }} / {{ store.roomConfig.maxPlayers }}</strong><span>{{ readyCount }} 人已准备</span></div>
          <template v-for="entry in seats" :key="entry.seat">
            <article v-if="entry.player" class="waiting-seat" :class="[`seat-${entry.seat}`, { speaking: entry.player.isSpeaking, local: entry.player.isLocal, disconnected: entry.player.status === 'disconnected' }]">
              <span class="waiting-avatar">{{ entry.player.initials }}</span><strong>{{ entry.player.name }}</strong><VoiceMeter v-if="entry.player.isSpeaking" active /><small>{{ entry.player.status === "disconnected" ? "已断线" : entry.player.isReady ? "已准备" : entry.player.isMuted ? "已禁言" : "等待准备" }}</small>
            </article>
            <button v-else class="waiting-seat empty" :class="`seat-${entry.seat}`" type="button" disabled><UserPlus /><span>{{ entry.available ? "空位" : "未开放" }}</span></button>
          </template>
        </div>
      </section>
      <aside class="waiting-console">
        <section><header><h2>牌局设置</h2><Settings2 /></header><dl><div><dt>人数上限</dt><dd>{{ store.roomConfig.maxPlayers }} 人</dd></div><div><dt>盲注</dt><dd>{{ store.roomConfig.blindPreset }}</dd></div><div><dt>行动时间</dt><dd>{{ store.roomConfig.actionSeconds }} 秒</dd></div><div><dt>筹码面额</dt><dd>{{ store.roomConfig.chipDenominations.join(" / ") }}</dd></div></dl></section>
        <section><header><h2>桌内语音</h2><Headphones /></header><button class="voice-test" type="button" :disabled="store.voiceBusy || !store.roomConfig.voiceEnabled" @click="store.toggleMicrophone"><span :class="{ live: store.microphoneEnabled }"><Mic v-if="store.microphoneEnabled" /><MicOff v-else /></span><span><strong>{{ !store.roomConfig.voiceEnabled ? "房间未开启语音" : store.voiceBusy ? "正在连接" : store.microphoneEnabled ? "麦克风正常" : "麦克风已关闭" }}</strong><small>{{ store.voiceError || (store.microphoneEnabled ? (store.voiceTransport === "livekit" ? "LiveKit 云端语音已连接" : "浏览器直连语音已连接") : "点击开启") }}</small></span><VoiceMeter :active="store.microphoneEnabled" /></button><select v-if="store.microphones.length > 1" class="device-select" aria-label="麦克风设备" :value="store.selectedMicrophoneId" @change="selectMicrophone"><option v-for="device in store.microphones" :key="device.deviceId" :value="device.deviceId">{{ device.label }}</option></select></section>
        <section class="invite-link"><header><h2>邀请链接</h2><Link2 /></header><code>{{ inviteUrl }}</code></section>
        <RoomManagementPanel v-if="isOwner" @room-ended="router.push('/')" />
        <p v-if="actionError" class="form-message error">{{ actionError }}</p>
        <button class="ready-button" :class="{ ready }" type="button" :disabled="store.commandPending" @click="toggleReady"><Check v-if="ready" /><span><strong>{{ store.commandPending ? "正在更新" : ready ? "已准备" : "准备入局" }}</strong><small>{{ ready ? "等待房主开始" : "确认牌局设置和语音状态" }}</small></span></button>
        <button class="button primary wide" type="button" :disabled="!ready || !isOwner || readyCount < 2 || !store.backendOnline" @click="startGame"><Play />{{ isOwner ? (readyCount < 2 ? "等待至少两人准备" : "开始牌局") : "等待房主开局" }}</button>
      </aside>
    </main>
  </div>
</template>
