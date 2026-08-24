<script setup lang="ts">
import { ArrowLeft, ArrowRight, Clock3, Headphones, ShieldCheck, Users } from "@lucide/vue";
import type { RoomConfig } from "@royal-flush/contracts";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import BrandMark from "@/components/BrandMark.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { api, apiMode } from "@/lib/api";
import { useGameStore } from "@/stores/game";

const route = useRoute();
const router = useRouter();
const store = useGameStore();
const step = ref<"phone" | "code">("phone");
const phone = ref("");
const code = ref("");
const error = ref("");
const busy = ref(false);
const devCode = ref("");
const roomCode = computed(() => String(route.params.code || "").toUpperCase());
const roomInfo = ref<{ id: string; code: string; name: string; ownerName: string; onlinePlayers: number; maxPlayers: number; occupiedSeats: number[]; config: RoomConfig } | null>(null);

async function requestCode() {
  if (!/^1[3-9]\d{9}$/.test(phone.value)) {
    error.value = "请输入 11 位中国大陆手机号";
    return;
  }
	busy.value = true;
	error.value = "";
	try {
		if (!apiMode) throw new Error("服务暂时不可用，请稍后重试");
		const challenge = await api.requestOtp(phone.value);
		devCode.value = challenge.devCode ?? "";
		step.value = "code";
	} catch (reason) {
		error.value = reason instanceof Error ? reason.message : "验证码发送失败";
	} finally {
		busy.value = false;
	}
}

async function verify() {
  if (!/^\d{6}$/.test(code.value)) {
    error.value = "请输入 6 位验证码";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    if (!apiMode) throw new Error("服务暂时不可用，请稍后重试");
    await api.verifyOtp(phone.value, code.value);
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
      <div class="auth-panel">
        <ShieldCheck class="auth-icon" />
        <h2>{{ step === 'phone' ? '手机号登录' : '输入验证码' }}</h2>
        <p>{{ step === 'phone' ? '登录后自动回到这个房间' : `验证码已发送到 ${phone}` }}</p>
        <form v-if="step === 'phone'" @submit.prevent="requestCode"><label for="phone">手机号</label><input id="phone" v-model="phone" inputmode="tel" autocomplete="tel" maxlength="11" placeholder="138 0000 0000" /><p v-if="error" class="form-message error">{{ error }}</p><button class="button primary wide" type="submit" :disabled="busy">{{ busy ? "正在发送" : "获取验证码" }}<ArrowRight /></button></form>
        <form v-else @submit.prevent="verify"><label for="otp">验证码</label><input id="otp" v-model="code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="000000" /><p v-if="devCode" class="dev-code">预览环境验证码：{{ devCode }}</p><p v-if="error" class="form-message error">{{ error }}</p><button class="button primary wide" type="submit" :disabled="busy">{{ busy ? "正在进入" : "验证并进入" }}<ArrowRight /></button><button class="text-button" type="button" @click="step = 'phone'; error = ''">更换手机号</button></form>
      </div>
      <p class="auth-legal">登录即表示同意用户协议与隐私政策。积分仅用于娱乐计分，不具备货币价值。</p>
    </section>
  </main>
</template>
