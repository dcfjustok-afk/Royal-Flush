<script setup lang="ts">
import { ArrowRight, Clock3, Copy, Headphones, Plus, Radio, Users } from "@lucide/vue";
import { ref } from "vue";
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
  await navigator.clipboard.writeText("RF-2806").catch(() => undefined);
}
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

        <div class="active-room-band">
          <div class="room-signal"><VoiceMeter active /><span>牌局进行中</span></div>
          <div class="active-room-name"><strong>周六夜场</strong><span>RF-2806 · 第 28 手牌</span></div>
          <dl class="room-facts"><div><dt>在线</dt><dd><Users />6 / 8</dd></div><div><dt>盲注</dt><dd>5 / 10</dd></div><div><dt>桌内语音</dt><dd><Headphones />已连接</dd></div></dl>
          <div class="room-band-actions"><button class="icon-button" type="button" title="复制房间码" aria-label="复制房间码" @click="copyCode"><Copy /></button><RouterLink class="button light" to="/rooms/room-saturday/table">返回牌桌<ArrowRight /></RouterLink></div>
        </div>
      </section>

      <div class="lobby-grid">
        <section class="join-section">
          <header><h2>加入好友牌局</h2><span>输入邀请中的房间码</span></header>
          <form class="join-form" @submit.prevent="joinRoom"><label for="room-code">房间码</label><div><input id="room-code" v-model="roomCode" autocomplete="off" maxlength="7" placeholder="RF-2806" aria-describedby="join-error" /><button class="button primary" type="submit">进入</button></div><p v-if="joinError" id="join-error" class="form-message error">{{ joinError }}</p></form>
          <div class="recent-rooms">
            <button type="button" @click="router.push('/rooms/room-friday/waiting')"><span class="recent-icon"><Clock3 /></span><span><strong>周五练习局</strong><small>3 天前 · 4 位好友</small></span><ArrowRight /></button>
            <button type="button" @click="router.push('/rooms/room-sunday/waiting')"><span class="recent-icon"><Radio /></span><span><strong>慢速夜场</strong><small>7 天前 · 6 位好友</small></span><ArrowRight /></button>
          </div>
        </section>

        <section class="score-section">
          <header><h2>局外积分</h2><RouterLink to="/profile">查看账本<ArrowRight /></RouterLink></header>
          <div class="large-score"><strong>{{ store.accountPoints.toLocaleString("zh-CN") }}</strong><span>账号共享余额</span></div>
          <ScoreAddPanel />
        </section>
      </div>

      <section class="activity-strip">
        <header><h2>最近信号</h2><span>所有示例数据均为本地演示</span></header>
        <ol><li v-for="message in store.messages.slice(0, 3)" :key="message.id"><time>{{ message.at }}</time><span :class="`event-dot ${message.type}`" /><p>{{ message.text }}</p></li></ol>
      </section>
    </main>
  </div>
</template>

