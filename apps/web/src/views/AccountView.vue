<script setup lang="ts">
import { ArrowLeft, Fingerprint, Radio, ShieldCheck, Volume2 } from "@lucide/vue";
import { onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import AccountAccessPanel from "@/components/AccountAccessPanel.vue";
import BrandMark from "@/components/BrandMark.vue";
import ThemeSwitcher from "@/components/ThemeSwitcher.vue";
import VoiceMeter from "@/components/VoiceMeter.vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const route = useRoute();
const router = useRouter();

function destination() {
  const redirect = typeof route.query.redirect === "string" ? route.query.redirect : "/";
  return redirect.startsWith("/") && !redirect.startsWith("//") ? redirect : "/";
}

async function authenticated() {
  await router.replace(destination());
}

onMounted(async () => {
  await store.probeBackend();
  await store.refreshAccount().catch(() => undefined);
  if (store.currentUser) await authenticated();
});
</script>

<template>
  <main id="main-content" class="account-page">
    <header class="invite-header"><BrandMark /><div class="invite-header-actions"><ThemeSwitcher compact /><RouterLink to="/"><ArrowLeft />返回首页</RouterLink></div></header>
    <section class="account-manifesto">
      <div class="account-live-label"><VoiceMeter active /><span>玩家身份工作台</span></div>
      <h1>一份身份，<br /><em>留住每一局。</em></h1>
      <p>积分账本、当前房间与牌桌昵称绑定到同一个玩家账号。语音只在房间内实时传输，不录制、不保存。</p>
      <dl>
        <div><dt><Fingerprint />持久身份</dt><dd>跨重启保留</dd></div>
        <div><dt><Radio />实时牌局</dt><dd>断线可恢复</dd></div>
        <div><dt><Volume2 />桌内语音</dt><dd>默认不录制</dd></div>
      </dl>
      <div class="account-trust-line"><ShieldCheck /><span>密码哈希、HttpOnly 会话、封禁状态实时校验</span></div>
    </section>
    <section class="account-access"><AccountAccessPanel @authenticated="authenticated" /><p class="auth-legal">注册或登录即表示同意用户协议与隐私政策。积分仅用于娱乐计分，不具备货币价值。</p></section>
  </main>
</template>
