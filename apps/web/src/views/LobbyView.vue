<script setup lang="ts">
import { ArrowRight, Copy, Headphones, Plus, Users } from "@lucide/vue";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import AppHeader from "@/components/AppHeader.vue";
import ScoreAddPanel from "@/components/ScoreAddPanel.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const router = useRouter();
const roomCode = ref("");
const joinError = ref("");

function joinRoom() {
  const code = roomCode.value.trim().toUpperCase();
  if (!/^RF-[A-Z0-9]{4}$/.test(code)) {
    joinError.value = "房间码格式应为 RF- 加四位字母或数字";
    return;
  }
  joinError.value = "";
  router.push(`/invite/${code}`);
}

async function copyCode() {
  if (store.snapshot.roomCode) await navigator.clipboard.writeText(store.snapshot.roomCode).catch(() => undefined);
}

const hasActiveRoom = computed(() => Boolean(store.activeRoomId && store.snapshot.roomId));

onMounted(async () => {
  await store.probeBackend();
  await store.refreshAccount().catch(() => undefined);
});
</script>

<template>
  <div class="page-shell lobby-page">
    <AppHeader />
    <main class="page-content">
      <section class="lobby-command">
        <div class="lobby-heading">
          <div><h1>今晚的牌局</h1><p>好友桌、桌内语音和积分记录都在这里。</p></div>
          <RouterLink class="button primary" to="/rooms/new"><Plus />创建牌局</RouterLink>
        </div>

        <div v-if="hasActiveRoom" class="active-room-band">
          <div class="room-signal"><VoiceMeter active /><span>牌局进行中</span></div>
          <div class="active-room-name"><strong>{{ store.snapshot.roomName }}</strong><span>{{ store.snapshot.roomCode }} · 第 {{ store.snapshot.handNumber }} 手牌</span></div>
          <dl class="room-facts"><div><dt>在线</dt><dd><Users />{{ store.activePlayers }} / {{ store.roomConfig.maxPlayers }}</dd></div><div><dt>盲注</dt><dd>{{ store.roomConfig.blindPreset }}</dd></div><div><dt>桌内语音</dt><dd><Headphones />{{ store.roomConfig.voiceEnabled ? "已开启" : "未开启" }}</dd></div></dl>
          <div class="room-band-actions"><button class="icon-button" type="button" title="复制房间码" aria-label="复制房间码" @click="copyCode"><Copy /></button><RouterLink class="button light" :to="`/rooms/${store.snapshot.roomId}/table`">返回牌桌<ArrowRight /></RouterLink></div>
        </div>
        <div v-else class="active-room-band"><div class="active-room-name"><strong>当前没有进行中的牌局</strong><span>创建房间或使用好友发来的房间码加入</span></div></div>
      </section>

      <div class="lobby-grid">
        <section class="join-section">
          <header><h2>加入好友牌局</h2><span>输入邀请中的房间码</span></header>
          <form class="join-form" @submit.prevent="joinRoom"><label for="room-code">房间码</label><div><input id="room-code" v-model="roomCode" autocomplete="off" maxlength="7" placeholder="RF-XXXX" aria-describedby="join-error" /><button class="button primary" type="submit">进入</button></div><p v-if="joinError" id="join-error" class="form-message error">{{ joinError }}</p></form>
        </section>

        <section class="score-section">
          <header><h2>局外积分</h2><RouterLink to="/profile">查看账本<ArrowRight /></RouterLink></header>
          <div class="large-score"><strong>{{ store.accountPoints?.toLocaleString("zh-CN") ?? "--" }}</strong><span>账号共享余额</span></div>
          <ScoreAddPanel />
        </section>
      </div>

      <section class="activity-strip">
        <header><h2>最近信号</h2><span>房间系统消息</span></header>
        <ol><li v-for="message in store.messages.slice(0, 3)" :key="message.id"><time>{{ message.at }}</time><span :class="`event-dot ${message.type}`" /><p>{{ message.text }}</p></li><li v-if="!store.messages.length"><p>暂无牌局消息</p></li></ol>
      </section>
    </main>
  </div>
</template>
