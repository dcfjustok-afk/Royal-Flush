<script setup lang="ts">
import { ArrowLeft, ArrowUpRight, CalendarDays, Radio, RotateCcw } from "@lucide/vue";
import { onMounted } from "vue";
import AppHeader from "@/components/AppHeader.vue";
import ScoreAddPanel from "@/components/ScoreAddPanel.vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const typeLabels = { initial_base: "初始积分", self_add: "自行增加", game_settlement: "牌局结算", admin_reset: "全站重置" } as const;

onMounted(async () => {
  await store.probeBackend();
  await store.refreshAccount().catch(() => undefined);
});
</script>

<template>
  <div class="page-shell profile-page">
    <AppHeader />
    <main class="form-page-content">
      <RouterLink class="back-link" to="/"><ArrowLeft />返回牌局大厅</RouterLink>
      <header class="profile-heading"><div><h1>积分账本</h1><p>所有积分变更均可追溯，不具备货币价值。</p></div><div class="profile-score"><span>当前局外积分</span><strong>{{ store.accountPoints.toLocaleString("zh-CN") }}</strong></div></header>
      <div class="profile-grid">
        <section class="ledger-section"><header><h2>变更记录</h2><span>{{ store.ledger.length }} 条记录</span></header><div class="ledger-table" role="table" aria-label="积分变更记录"><div class="ledger-row ledger-head" role="row"><span>时间</span><span>类型</span><span>说明</span><span>变更</span><span>余额</span></div><div v-for="entry in store.ledger" :key="entry.id" class="ledger-row" role="row"><time>{{ new Date(entry.createdAt).toLocaleDateString("zh-CN") }}</time><span><i :class="entry.type"><Radio v-if="entry.type === 'game_settlement'" /><RotateCcw v-else-if="entry.type === 'admin_reset'" /><ArrowUpRight v-else /></i>{{ typeLabels[entry.type] }}</span><span>{{ entry.note }}</span><strong :class="{ negative: entry.amount < 0 }">{{ entry.amount > 0 ? "+" : "" }}{{ entry.amount.toLocaleString("zh-CN") }}</strong><b>{{ entry.balance.toLocaleString("zh-CN") }}</b></div></div></section>
        <aside class="profile-side"><ScoreAddPanel /><section class="account-facts"><header><h2>账号</h2><CalendarDays /></header><dl><div><dt>显示名称</dt><dd>岱奇</dd></div><div><dt>手机号</dt><dd>138 **** 2806</dd></div><div><dt>当前积分周期</dt><dd>Epoch 4</dd></div></dl></section></aside>
      </div>
    </main>
  </div>
</template>
