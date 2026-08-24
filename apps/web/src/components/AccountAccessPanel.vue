<script setup lang="ts">
import { Eye, EyeOff, LogIn, ShieldCheck, UserPlus } from "@lucide/vue";
import { onMounted, ref } from "vue";
import { useGameStore } from "@/stores/game";

const props = withDefaults(defineProps<{ initialMode?: "login" | "register" }>(), { initialMode: "login" });
const emit = defineEmits<{ authenticated: [] }>();
const store = useGameStore();
const mode = ref<"login" | "register">(props.initialMode);
const phone = ref("");
const password = ref("");
const nickname = ref("");
const showPassword = ref(false);
const error = ref("");
const busy = ref(false);

function setMode(next: "login" | "register") {
  mode.value = next;
  error.value = "";
}

async function submit() {
  error.value = "";
  if (!/^1[3-9]\d{9}$/.test(phone.value.trim())) {
    error.value = "请输入 11 位中国大陆手机号";
    return;
  }
  if (mode.value === "register" && !nickname.value.trim()) {
    error.value = "请填写牌桌显示名称";
    return;
  }
  if (password.value.length < 8 || !/[A-Za-z]/.test(password.value) || !/\d/.test(password.value)) {
    error.value = "密码至少 8 位，并同时包含字母和数字";
    return;
  }
  busy.value = true;
  try {
    if (!store.backendOnline) await store.probeBackend();
    if (mode.value === "register") await store.registerAccount(phone.value.trim(), password.value, nickname.value.trim());
    else await store.loginAccount(phone.value.trim(), password.value);
    emit("authenticated");
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "账号操作失败";
  } finally {
    busy.value = false;
  }
}

onMounted(() => store.probeBackend());
</script>

<template>
  <section class="auth-panel account-access-panel" aria-labelledby="account-access-title">
    <div class="auth-signal"><ShieldCheck /><span>身份与积分将跨设备保留</span></div>
    <div class="auth-mode-switch" role="tablist" aria-label="账号操作">
      <button type="button" role="tab" :aria-selected="mode === 'login'" :class="{ active: mode === 'login' }" @click="setMode('login')"><LogIn />登录</button>
      <button type="button" role="tab" :aria-selected="mode === 'register'" :class="{ active: mode === 'register' }" @click="setMode('register')"><UserPlus />注册</button>
    </div>
    <h2 id="account-access-title">{{ mode === "login" ? "回到你的牌桌" : "建立玩家账号" }}</h2>
    <p>{{ mode === "login" ? "使用手机号与密码恢复积分、房间和身份。" : "创建后即可加入邀请房间，账号不会因服务重启丢失。" }}</p>
    <form @submit.prevent="submit">
      <label v-if="mode === 'register'" for="account-nickname">显示名称</label>
      <input v-if="mode === 'register'" id="account-nickname" v-model="nickname" autocomplete="nickname" maxlength="20" placeholder="例如：小北" />
      <label for="account-phone">手机号</label>
      <input id="account-phone" v-model="phone" inputmode="tel" autocomplete="tel" maxlength="11" placeholder="138 0000 0000" />
      <label for="account-password">密码</label>
      <div class="password-field">
        <input id="account-password" v-model="password" :type="showPassword ? 'text' : 'password'" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" maxlength="72" placeholder="至少 8 位，包含字母与数字" />
        <button type="button" :aria-label="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword"><EyeOff v-if="showPassword" /><Eye v-else /></button>
      </div>
      <p v-if="error" class="form-message error" role="alert">{{ error }}</p>
      <button class="button primary wide" type="submit" :disabled="busy || !store.backendOnline">
        {{ busy ? "正在确认身份" : mode === "login" ? "登录账号" : "注册并登录" }}
        <LogIn v-if="mode === 'login'" /><UserPlus v-else />
      </button>
    </form>
    <p class="account-security-note">密码仅保存不可逆哈希；登录凭证使用 HttpOnly Cookie，不写入浏览器脚本存储。</p>
  </section>
</template>
