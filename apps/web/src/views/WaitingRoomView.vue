<script setup lang="ts">
import { Check, Copy, Headphones, Link2, Mic, MicOff, Play, Settings2, UserPlus } from "@lucide/vue";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import AppHeader from "@/components/AppHeader.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { apiMode } from "@/lib/api";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const route = useRoute();
const router = useRouter();
const ready = ref(false);
const copied = ref(false);
const actionError = ref("");
const inviteUrl = computed(() => `${location.origin}/invite/${store.snapshot.roomCode}`);
const isOwner = computed(() => !store.snapshot.ownerId || store.snapshot.ownerId === store.localPlayer?.id);

async function copyInvite() {
  await navigator.clipboard.writeText(inviteUrl.value).catch(() => undefined);
  copied.value = true;
  window.setTimeout(() => (copied.value = false), 1800);
}

async function toggleReady() {
  const next = !ready.value;
  if (!apiMode || !store.backendOnline) {
    ready.value = next;
    return;
  }
  try {
    await store.sendCommand("room.ready", { ready: next });
    ready.value = next;
  } catch (reason) {
    actionError.value = reason instanceof Error ? reason.message : "更新准备状态失败";
  }
}

async function startGame() {
  try {
    if (apiMode && store.backendOnline) await store.sendCommand("game.start");
    await router.push(`/rooms/${store.snapshot.roomId}/table`);
  } catch (reason) {
    actionError.value = reason instanceof Error ? reason.message : "无法开始牌局";
  }
}

onMounted(async () => {
  await store.probeBackend();
  if (!apiMode || !store.backendOnline) return;
  try {
    const snapshot = await store.loadRoom(String(route.params.id));
    ready.value = Boolean(snapshot.players.find((player) => player.isLocal)?.isReady);
    store.connectRoomEvents(snapshot.roomId);
  } catch (reason) {
    actionError.value = reason instanceof Error ? reason.message : "无法恢复房间状态";
  }
});
</script>

<template>
  <div class="page-shell waiting-page">
    <AppHeader />
    <main class="waiting-content">
      <header class="waiting-heading"><div><span class="live-label"><VoiceMeter active />好友正在加入</span><h1>{{ store.roomConfig.name }}</h1><p>{{ store.snapshot.roomCode }} · {{ isOwner ? "你是房主" : "等待房主开局" }}</p></div><button class="button light" type="button" @click="copyInvite"><Check v-if="copied" /><Copy v-else />{{ copied ? "已复制" : "复制邀请链接" }}</button></header>
      <section class="waiting-table-area">
        <div class="waiting-table">
          <div class="waiting-table-center"><strong>3 / {{ store.roomConfig.maxPlayers }}</strong><span>已入座</span></div>
          <article class="waiting-seat seat-0 speaking"><span class="waiting-avatar">桥</span><strong>阿桥</strong><VoiceMeter active /><small>已准备</small></article>
          <article class="waiting-seat seat-2"><span class="waiting-avatar">北</span><strong>小北</strong><small>设置语音中</small></article>
          <article class="waiting-seat seat-5 local"><span class="waiting-avatar">你</span><strong>你</strong><small>{{ ready ? "已准备" : "等待准备" }}</small></article>
          <button v-for="seat in [1,3,4,6,7]" :key="seat" class="waiting-seat empty" :class="`seat-${seat}`" type="button"><UserPlus /><span>空位</span></button>
        </div>
      </section>
      <aside class="waiting-console">
        <section><header><h2>牌局设置</h2><Settings2 /></header><dl><div><dt>人数上限</dt><dd>{{ store.roomConfig.maxPlayers }} 人</dd></div><div><dt>盲注</dt><dd>{{ store.roomConfig.blindPreset }}</dd></div><div><dt>行动时间</dt><dd>{{ store.roomConfig.actionSeconds }} 秒</dd></div><div><dt>筹码面额</dt><dd>{{ store.roomConfig.chipDenominations.join(" / ") }}</dd></div></dl></section>
        <section><header><h2>桌内语音</h2><Headphones /></header><button class="voice-test" type="button" @click="store.toggleMicrophone"><span :class="{ live: store.microphoneEnabled }"><Mic v-if="store.microphoneEnabled" /><MicOff v-else /></span><span><strong>{{ store.microphoneEnabled ? "麦克风正常" : "麦克风已关闭" }}</strong><small>{{ store.microphoneEnabled ? "输入设备：系统默认" : "点击重新开启" }}</small></span><VoiceMeter :active="store.microphoneEnabled" /></button></section>
        <section class="invite-link"><header><h2>邀请链接</h2><Link2 /></header><code>{{ inviteUrl }}</code></section>
        <p v-if="actionError" class="form-message error">{{ actionError }}</p>
        <button class="ready-button" :class="{ ready }" type="button" @click="toggleReady"><Check v-if="ready" /><span><strong>{{ ready ? "已准备" : "准备入局" }}</strong><small>{{ ready ? "等待房主开始" : "确认牌局设置和语音状态" }}</small></span></button>
        <button class="button primary wide" type="button" :disabled="!ready || !isOwner" @click="startGame"><Play />{{ isOwner ? "开始牌局" : "等待房主开局" }}</button>
      </aside>
    </main>
  </div>
</template>
