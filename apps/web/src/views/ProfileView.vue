<script setup lang="ts">
import { ArrowLeft, ArrowUpRight, CalendarDays, LogOut, Radio, RotateCcw, Save, UserRound } from "@lucide/vue";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import AppHeader from "@/components/AppHeader.vue";
import ScoreAddPanel from "@/components/ScoreAddPanel.vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const router = useRouter();
const nickname = ref("");
const message = ref("");
const busy = ref(false);
const typeLabels = { initial_base: "初始积分", self_add: "自行增加", game_settlement: "牌局结算", admin_reset: "全站重置" } as const;
const maskedPhone = computed(() => store.currentUser?.phone ? `${store.currentUser.phone.slice(0, 3)}****${store.currentUser.phone.slice(-4)}` : "--");

async function saveProfile() {
  message.value = "";
  busy.value = true;
  try {
    await store.updateNickname(nickname.value);
    message.value = "显示名称已更新";
  } catch (reason) {
    message.value = reason instanceof Error ? reason.message : "账号资料更新失败";
  } finally {
    busy.value = false;
  }
}

async function logout() {
  busy.value = true;
  try {
    await store.logoutAccount();
    await router.replace("/account");
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  await store.probeBackend();
  await store.refreshAccount().catch(() => undefined);
  if (!store.currentUser) {
    await router.replace("/account?redirect=/profile");
    return;
  }
  nickname.value = store.currentUser.nickname;
});
</script>

<template>
  <div class="page-shell profile-page">
    <AppHeader />
    <main id="main-content" class="form-page-content">
      <RouterLink class="back-link" to="/"><ArrowLeft />返回牌局大厅</RouterLink>
      <header class="profile-heading"><div><h1>积分账本</h1><p>账号、积分和当前房间均由服务端持久保存。</p></div><div class="profile-score"><span>当前局外积分</span><strong>{{ store.accountPoints?.toLocaleString("zh-CN") ?? "--" }}</strong></div></header>
      <div class="profile-grid">
        <section class="ledger-section"><header><h2>变更记录</h2><span>{{ store.ledger.length }} 条记录</span></header><div class="ledger-table" role="table" aria-label="积分变更记录"><div class="ledger-row ledger-head" role="row"><span>时间</span><span>类型</span><span>说明</span><span>变更</span><span>余额</span></div><div v-for="entry in store.ledger" :key="entry.id" class="ledger-row" role="row"><time>{{ new Date(entry.createdAt).toLocaleDateString("zh-CN") }}</time><span><i :class="entry.type"><Radio v-if="entry.type === 'game_settlement'" /><RotateCcw v-else-if="entry.type === 'admin_reset'" /><ArrowUpRight v-else /></i>{{ typeLabels[entry.type] }}</span><span>{{ entry.note }}</span><strong :class="{ negative: entry.amount < 0 }">{{ entry.amount > 0 ? "+" : "" }}{{ entry.amount.toLocaleString("zh-CN") }}</strong><b>{{ entry.balance.toLocaleString("zh-CN") }}</b></div><div v-if="!store.ledger.length" class="ledger-empty" role="row"><CalendarDays /><span>暂无积分变更记录</span></div></div></section>
        <aside v-if="store.currentUser" class="profile-side">
          <ScoreAddPanel />
          <section class="account-facts"><header><h2>账号档案</h2><UserRound /></header><dl><div><dt>手机号</dt><dd>{{ maskedPhone }}</dd></div><div><dt>账号 ID</dt><dd>{{ store.currentUser.id }}</dd></div><div><dt>注册时间</dt><dd>{{ store.currentUser.createdAt ? new Date(store.currentUser.createdAt).toLocaleDateString("zh-CN") : "--" }}</dd></div></dl></section>
          <form class="profile-account-form" @submit.prevent="saveProfile"><label for="profile-nickname">牌桌显示名称</label><div><input id="profile-nickname" v-model="nickname" name="nickname" maxlength="20" autocomplete="nickname" /><button class="button light" type="submit" :disabled="busy || nickname.trim() === store.currentUser.nickname"><Save />保存名称</button></div><p v-if="message" class="form-message" :class="{ error: !message.includes('已更新'), success: message.includes('已更新') }" aria-live="polite">{{ message }}</p></form>
          <button class="account-logout" type="button" :disabled="busy" @click="logout"><LogOut /><span><strong>退出当前账号</strong><small>此设备上的会话将立即失效</small></span></button>
        </aside>
      </div>
    </main>
  </div>
</template>
