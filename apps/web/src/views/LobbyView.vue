<script setup lang="ts">
import { ArrowRight, CircleUserRound, Copy, Headphones, Mic, Plus, Radio, Users, Wifi } from "@lucide/vue";
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
const copyFeedback = ref("");

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
	if (!store.snapshot.roomCode) return;
	copyFeedback.value = "";
	try {
		await navigator.clipboard.writeText(store.snapshot.roomCode);
		copyFeedback.value = "房间码已复制";
		window.setTimeout(() => (copyFeedback.value = ""), 1800);
	} catch {
		copyFeedback.value = "复制失败，请手动记录房间码";
	}
}

const hasActiveRoom = computed(() => Boolean(store.activeRoomId && store.snapshot.roomId === store.activeRoomId && store.localPlayer));
const activeRoomRoute = computed(() => store.activeRoomRoute());
const activeRoomStatus = computed(() => store.snapshot.street === "waiting" ? "等待玩家准备" : store.snapshot.street === "settled" ? "等待下一手" : "牌局进行中");
const activeRoomAction = computed(() => store.snapshot.street === "waiting" ? "返回等候室" : "返回牌桌");
const lobbyStatuses = computed(() => [
  {
    id: "service",
    icon: Wifi,
    label: "牌局服务",
    value: store.backendOnline ? "连接正常" : "等待恢复",
    tone: store.backendOnline ? "live" : "waiting",
  },
  {
    id: "account",
    icon: CircleUserRound,
    label: "账号同步",
    value: store.currentUser ? `已同步 · ${store.currentUser.nickname}` : "等待登录",
    tone: store.currentUser ? "live" : "waiting",
  },
  {
    id: "voice",
    icon: Mic,
    label: "桌内语音",
    value: hasActiveRoom.value
      ? store.roomConfig.voiceEnabled
        ? store.microphoneEnabled ? "麦克风已开启" : "房间已开放"
        : "本房间未开启"
      : "随房间配置",
    tone: hasActiveRoom.value && store.roomConfig.voiceEnabled ? "voice" : "neutral",
  },
]);
const activityItems = computed(() => {
  if (store.messages.length) return store.messages.slice(0, 4);
  return [
    {
      id: "service-status",
      type: store.backendOnline ? "action" : "waiting",
      at: "现在",
      text: store.backendOnline ? "牌局服务已连接" : "牌局服务暂时离线，等待恢复",
    },
    {
      id: "account-status",
      type: store.currentUser ? "action" : "waiting",
      at: "现在",
      text: store.currentUser ? `${store.currentUser.nickname} 的账号与局外积分已同步` : "尚未登录账号",
    },
    {
      id: "room-status",
      type: hasActiveRoom.value ? "score" : "neutral",
      at: "待命",
      text: hasActiveRoom.value ? `${store.snapshot.roomName} 正在等待你返回` : "暂无房间系统消息",
    },
  ];
});

onMounted(async () => {
  await store.probeBackend();
  await store.refreshAccount().catch(() => undefined);
});
</script>

<template>
  <div class="page-shell lobby-page">
    <AppHeader />
    <main id="main-content" class="page-content">
      <section class="lobby-command">
        <div class="lobby-heading">
          <div><h1>今晚的牌局</h1><p>好友桌、桌内语音和积分记录都在这里。</p></div>
          <RouterLink class="button primary" :to="store.currentUser ? '/rooms/new' : '/account?redirect=/rooms/new'"><Plus />{{ store.currentUser ? "创建牌局" : "登录后创建" }}</RouterLink>
        </div>

        <div v-if="hasActiveRoom" class="active-room-band">
          <div class="room-signal"><VoiceMeter active /><span>{{ activeRoomStatus }}</span></div>
          <div class="active-room-name"><strong>{{ store.snapshot.roomName }}</strong><span>{{ store.snapshot.roomCode }} · 第 {{ store.snapshot.handNumber }} 手牌</span></div>
          <dl class="room-facts"><div><dt>在线</dt><dd><Users />{{ store.activePlayers }} / {{ store.roomConfig.maxPlayers }}</dd></div><div><dt>盲注</dt><dd>{{ store.roomConfig.blindPreset }}</dd></div><div><dt>桌内语音</dt><dd><Headphones />{{ store.roomConfig.voiceEnabled ? "已开启" : "未开启" }}</dd></div></dl>
          <div class="room-band-actions"><span v-if="copyFeedback" role="status">{{ copyFeedback }}</span><button class="icon-button" type="button" title="复制房间码" aria-label="复制房间码" @click="copyCode"><Copy /></button><RouterLink class="button light" :to="activeRoomRoute">{{ activeRoomAction }}<ArrowRight /></RouterLink></div>
        </div>
        <div v-else class="active-room-band standby-room-band">
          <div class="standby-heading">
            <span class="standby-icon"><Radio /></span>
            <div><strong>牌局待命</strong><span>当前没有进行中的房间</span></div>
          </div>
          <dl class="standby-statuses">
            <div v-for="item in lobbyStatuses" :key="item.id">
              <dt><component :is="item.icon" />{{ item.label }}</dt>
              <dd :class="`status-${item.tone}`"><span />{{ item.value }}</dd>
            </div>
          </dl>
          <RouterLink class="button light" :to="store.currentUser ? '/rooms/new' : '/account?redirect=/rooms/new'">{{ store.currentUser ? "建立好友桌" : "登录账号" }}<ArrowRight /></RouterLink>
        </div>
      </section>

      <div class="lobby-workbench">
        <section class="lobby-module join-section">
          <header><div><h2>加入好友牌局</h2><p>使用好友分享的房间码进入等候室</p></div><span>邀请入口</span></header>
          <form class="join-form" @submit.prevent="joinRoom">
            <label for="room-code">房间码</label>
            <div><input id="room-code" v-model="roomCode" name="room-code" autocomplete="off" maxlength="7" placeholder="RF-XXXX" spellcheck="false" aria-describedby="join-error" /><button class="button primary" type="submit">进入房间<ArrowRight /></button></div>
            <p v-if="joinError" id="join-error" class="form-message error" role="alert">{{ joinError }}</p>
          </form>
          <div class="join-alternative"><span>还没有房间码？</span><RouterLink :to="store.currentUser ? '/rooms/new' : '/account?redirect=/rooms/new'">{{ store.currentUser ? "创建自己的牌局" : "登录后创建牌局" }}<Plus /></RouterLink></div>
        </section>

        <section class="lobby-module score-section">
          <header><div><h2>局外积分</h2><p>好友桌共享的娱乐计分</p></div><RouterLink to="/profile">查看账本<ArrowRight /></RouterLink></header>
          <div class="large-score"><strong>{{ store.accountPoints?.toLocaleString("zh-CN") ?? "--" }}</strong><span>账号共享余额</span></div>
          <ScoreAddPanel v-if="store.currentUser" />
          <RouterLink v-else class="account-required" to="/account"><strong>登录后管理积分</strong><span>账号余额与账本会跨设备保留</span><ArrowRight /></RouterLink>
        </section>
      </div>

      <section class="activity-strip">
        <header><div><h2>最近信号</h2><p>账号与房间的最新状态</p></div><span>{{ store.messages.length ? "房间系统消息" : "控制台状态" }}</span></header>
        <ol><li v-for="message in activityItems" :key="message.id"><time>{{ message.at }}</time><span :class="`event-dot ${message.type}`" /><p>{{ message.text }}</p></li></ol>
      </section>
    </main>
  </div>
</template>
