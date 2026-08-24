<script setup lang="ts">
import { ArrowLeft, ArrowRight, Clock3, Headphones, ShieldCheck, Users } from "@lucide/vue";
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import BrandMark from "@/components/BrandMark.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";

const route = useRoute();
const router = useRouter();
const step = ref<"phone" | "code">("phone");
const phone = ref("");
const code = ref("");
const error = ref("");
const roomCode = computed(() => String(route.params.code || "RF-2806").toUpperCase());

function requestCode() {
  if (!/^1\d{10}$/.test(phone.value)) {
    error.value = "请输入 11 位中国大陆手机号";
    return;
  }
  error.value = "";
  step.value = "code";
}

function verify() {
  if (!/^\d{6}$/.test(code.value)) {
    error.value = "请输入 6 位验证码";
    return;
  }
  router.push(`/rooms/${roomCode.value}/waiting`);
}
</script>

<template>
  <main class="invite-page">
    <header class="invite-header"><BrandMark /><RouterLink to="/"><ArrowLeft />返回首页</RouterLink></header>
    <section class="invite-room">
      <div class="invite-signal"><VoiceMeter active /><span>好友正在等你</span></div>
      <h1>周六夜场</h1>
      <p class="invite-code">{{ roomCode }}</p>
      <dl><div><dt><Users />当前人数</dt><dd>6 / 8</dd></div><div><dt><Clock3 />行动时间</dt><dd>30 秒</dd></div><div><dt><Headphones />桌内语音</dt><dd>已开启</dd></div></dl>
      <p class="room-owner">房主：阿桥 · 盲注 5 / 10</p>
    </section>
    <section class="invite-auth">
      <div class="auth-panel">
        <ShieldCheck class="auth-icon" />
        <h2>{{ step === 'phone' ? '手机号登录' : '输入验证码' }}</h2>
        <p>{{ step === 'phone' ? '登录后自动回到这个房间' : `验证码已发送到 ${phone}` }}</p>
        <form v-if="step === 'phone'" @submit.prevent="requestCode"><label for="phone">手机号</label><input id="phone" v-model="phone" inputmode="tel" autocomplete="tel" maxlength="11" placeholder="138 0000 0000" /><p v-if="error" class="form-message error">{{ error }}</p><button class="button primary wide" type="submit">获取验证码<ArrowRight /></button></form>
        <form v-else @submit.prevent="verify"><label for="otp">验证码</label><input id="otp" v-model="code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="000000" /><p class="dev-code">本地演示验证码：123456</p><p v-if="error" class="form-message error">{{ error }}</p><button class="button primary wide" type="submit">验证并进入<ArrowRight /></button><button class="text-button" type="button" @click="step = 'phone'; error = ''">更换手机号</button></form>
      </div>
      <p class="auth-legal">登录即表示同意用户协议与隐私政策。积分仅用于娱乐计分，不具备货币价值。</p>
    </section>
  </main>
</template>

