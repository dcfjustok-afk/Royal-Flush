<script setup lang="ts">
import { ArrowLeft, ArrowRight, CircleCheck, Clock3, Headphones, Users } from "@lucide/vue";
import type { RoomConfig } from "@royal-flush/contracts";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import AccountAccessPanel from "@/components/AccountAccessPanel.vue";
import BrandMark from "@/components/BrandMark.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { api, apiMode } from "@/lib/api";
import { useGameStore } from "@/stores/game";

const route = useRoute();
const router = useRouter();
const store = useGameStore();
const error = ref("");
const busy = ref(false);
const roomCode = computed(() => String(route.params.code || "").toUpperCase());
const roomInfo = ref<{ id: string; code: string; name: string; ownerName: string; onlinePlayers: number; maxPlayers: number; occupiedSeats: number[]; config: RoomConfig } | null>(null);

async function enterRoom() {
  busy.value = true;
  error.value = "";
  try {
    if (!apiMode || !store.currentUser) throw new Error("请先登录玩家账号");
    const info = roomInfo.value ?? await api.publicRoom(roomCode.value);
    const occupied = new Set(info.occupiedSeats);
    const seat = Array.from({ length: info.maxPlayers }, (_, index) => index).find((candidate) => !occupied.has(candidate));
    if (seat === undefined) throw new Error("房间已经坐满");
    const snapshot = await store.joinRoom(info.id, seat);
    await router.push(`/rooms/${snapshot.roomId}/waiting`);
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "登录或入座失败";
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  if (!apiMode) {
    error.value = "服务暂时不可用，请稍后重试";
    return;
  }
  await store.probeBackend();
  await store.refreshAccount().catch(() => undefined);
  try {
    roomInfo.value = await api.publicRoom(roomCode.value);
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "无法读取房间信息";
  }
});
</script>

<template>
  <main class="invite-page">
    <header class="invite-header"><BrandMark /><RouterLink to="/"><ArrowLeft />返回首页</RouterLink></header>
    <section class="invite-room">
      <div class="invite-signal"><VoiceMeter :active="Boolean(roomInfo)" /><span>{{ roomInfo ? "好友正在等你" : "正在读取房间" }}</span></div>
      <h1>{{ roomInfo?.name ?? "房间信息加载中" }}</h1>
      <p class="invite-code">{{ roomCode }}</p>
      <dl><div><dt><Users />当前人数</dt><dd>{{ roomInfo ? `${roomInfo.onlinePlayers} / ${roomInfo.maxPlayers}` : "--" }}</dd></div><div><dt><Clock3 />行动时间</dt><dd>{{ roomInfo ? `${roomInfo.config.actionSeconds} 秒` : "--" }}</dd></div><div><dt><Headphones />桌内语音</dt><dd>{{ roomInfo ? (roomInfo.config.voiceEnabled ? "已开启" : "未开启") : "--" }}</dd></div></dl>
      <p class="room-owner">{{ roomInfo ? `房主：${roomInfo.ownerName} · 盲注 ${roomInfo.config.blindPreset}` : "等待后端返回真实房间信息" }}</p>
    </section>
    <section class="invite-auth">
      <AccountAccessPanel v-if="!store.currentUser" @authenticated="enterRoom" />
      <div v-else class="auth-panel invite-signed-in">
        <CircleCheck class="auth-icon" />
        <h2>{{ store.currentUser.nickname }}，身份已确认</h2>
        <p>将使用账号积分加入 {{ roomInfo?.name ?? "这个房间" }}，入座后可先测试麦克风。</p>
        <p v-if="error" class="form-message error" role="alert">{{ error }}</p>
        <button class="button primary wide" type="button" :disabled="busy || !roomInfo" @click="enterRoom">{{ busy ? "正在入座" : "进入等候室" }}<ArrowRight /></button>
      </div>
      <p class="auth-legal">登录即表示同意用户协议与隐私政策。积分仅用于娱乐计分，不具备货币价值。</p>
    </section>
  </main>
</template>
