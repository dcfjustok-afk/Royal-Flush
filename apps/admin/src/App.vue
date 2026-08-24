<script setup lang="ts">
import { Activity, AlertTriangle, CheckCircle2, ChevronDown, CircleUserRound, Clock3, DoorOpen, FileClock, Gauge, Radio, RefreshCw, RotateCcw, Search, ShieldCheck, Users, X } from "@lucide/vue";
import { computed, ref } from "vue";

const activeSection = ref<"overview" | "users" | "rooms" | "reports" | "audit">("overview");
const search = ref("");
const resetOpen = ref(false);
const resetReason = ref("");
const confirmation = ref("");
const resetDone = ref(false);
const epoch = ref(4);

const rooms = [
  { id: "RF-2806", name: "周六夜场", owner: "岱奇", players: "6 / 8", blind: "5 / 10", hand: 28, voice: "正常", state: "进行中" },
  { id: "RF-9132", name: "慢速夜场", owner: "阿桥", players: "4 / 6", blind: "2 / 5", hand: 11, voice: "2 人关闭", state: "进行中" },
  { id: "RF-0475", name: "练习桌", owner: "小北", players: "2 / 8", blind: "10 / 20", hand: 0, voice: "正常", state: "等候中" },
];
const users = [
  { name: "岱奇", phone: "138 **** 2806", score: 1860, room: "RF-2806", status: "正常", updatedAt: "22:48" },
  { name: "阿桥", phone: "139 **** 1408", score: 3680, room: "RF-2806", status: "正常", updatedAt: "22:47" },
  { name: "远山", phone: "186 **** 9021", score: -240, room: "RF-2806", status: "正常", updatedAt: "22:45" },
  { name: "林度", phone: "137 **** 6110", score: 610, room: "RF-2806", status: "正常", updatedAt: "22:43" },
];
const audits = ref([
  { time: "2026-08-20 10:34", operator: "ops.li", action: "全站积分重置", detail: "Epoch 3 → 4，基线 1,000，季度演练", result: "成功" },
  { time: "2026-08-18 17:20", operator: "ops.chen", action: "用户解除封禁", detail: "用户 U-01842", result: "成功" },
]);

const filteredUsers = computed(() => users.filter((user) => `${user.name}${user.phone}${user.room}`.toLowerCase().includes(search.value.toLowerCase())));

function performReset() {
  if (confirmation.value !== "RESET ALL SCORES" || resetReason.value.trim().length < 4) return;
  epoch.value += 1;
  audits.value.unshift({ time: new Date().toLocaleString("zh-CN", { hour12: false }), operator: "ops.daichi", action: "全站积分重置", detail: `Epoch ${epoch.value - 1} → ${epoch.value}，基线 1,000，${resetReason.value.trim()}`, result: "成功" });
  resetDone.value = true;
  window.setTimeout(() => closeReset(), 1400);
}

function closeReset() {
  resetOpen.value = false;
  resetDone.value = false;
  resetReason.value = "";
  confirmation.value = "";
}
</script>

<template>
  <div class="admin-shell">
    <aside class="admin-sidebar">
      <a class="admin-brand" href="/"><span>RF</span><strong>Royal Flush<small>运营控制台</small></strong></a>
      <nav aria-label="运营导航">
        <button :class="{ active: activeSection === 'overview' }" @click="activeSection = 'overview'"><Gauge />运行概览</button>
        <button :class="{ active: activeSection === 'users' }" @click="activeSection = 'users'"><Users />用户与积分</button>
        <button :class="{ active: activeSection === 'rooms' }" @click="activeSection = 'rooms'"><DoorOpen />活跃房间</button>
        <button :class="{ active: activeSection === 'reports' }" @click="activeSection = 'reports'"><AlertTriangle />举报处理<span class="nav-count">2</span></button>
        <button :class="{ active: activeSection === 'audit' }" @click="activeSection = 'audit'"><FileClock />审计记录</button>
      </nav>
      <div class="admin-permission"><ShieldCheck /><span><strong>平台运营管理员</strong><small>score:reset-all</small></span></div>
    </aside>

    <main class="admin-main">
      <header class="admin-topbar"><div><h1>{{ { overview: '运行概览', users: '用户与积分', rooms: '活跃房间', reports: '举报处理', audit: '审计记录' }[activeSection] }}</h1><span>2026-08-24 · Asia/Shanghai</span></div><div class="operator"><span class="service-live">服务正常</span><button type="button"><CircleUserRound />ops.daichi<ChevronDown /></button></div></header>

      <template v-if="activeSection === 'overview'">
        <section class="summary-band" aria-label="运行摘要"><div><span>在线连接</span><strong>864</strong><small><Activity />过去 5 分钟稳定</small></div><div><span>活跃房间</span><strong>120</strong><small><Radio />91 桌正在出牌</small></div><div><span>语音参与</span><strong>72%</strong><small>LiveKit 正常</small></div><div><span>待处理举报</span><strong>2</strong><small class="warning">最早等待 18 分钟</small></div></section>
        <section class="operations-grid"><div class="operations-main"><header class="section-header"><div><h2>活跃房间</h2><span>按最近事件排序</span></div><button class="tool-button" type="button"><RefreshCw />刷新</button></header><div class="admin-table"><div class="admin-row head"><span>房间</span><span>房主</span><span>人数</span><span>盲注</span><span>手牌</span><span>语音</span><span>状态</span></div><div v-for="room in rooms" :key="room.id" class="admin-row"><span><strong>{{ room.name }}</strong><small>{{ room.id }}</small></span><span>{{ room.owner }}</span><span>{{ room.players }}</span><span>{{ room.blind }}</span><span># {{ String(room.hand).padStart(3, '0') }}</span><span>{{ room.voice }}</span><span><i :class="{ waiting: room.state === '等候中' }" />{{ room.state }}</span></div></div></div><aside class="operations-side"><section class="score-epoch"><header><h2>积分周期</h2><RotateCcw /></header><span>当前 Epoch</span><strong>{{ epoch }}</strong><p>全站基线 1,000，活跃牌局继续结算。</p><button class="danger-button" type="button" @click="resetOpen = true"><RotateCcw />重置全站积分</button></section><section class="system-feed"><header><h2>系统事件</h2><Clock3 /></header><ol><li><time>22:48</time><span>RF-2806 玩家自行增加 500 积分</span></li><li><time>22:43</time><span>LiveKit 节点丢包恢复正常</span></li><li><time>22:39</time><span>RF-9132 房主转移成功</span></li></ol></section></aside></section>
      </template>

      <template v-else-if="activeSection === 'users'">
        <section class="admin-workspace"><header class="workspace-tools"><label><Search /><input v-model="search" placeholder="搜索昵称、手机号或房间码" /></label><button class="danger-button" type="button" @click="resetOpen = true"><RotateCcw />重置全站积分</button></header><div class="admin-table users-table"><div class="admin-row head"><span>用户</span><span>手机号</span><span>局外积分</span><span>当前房间</span><span>状态</span><span>最后事件</span></div><div v-for="user in filteredUsers" :key="user.phone" class="admin-row"><span><strong>{{ user.name }}</strong></span><span>{{ user.phone }}</span><span :class="{ negative: user.score < 0 }">{{ user.score.toLocaleString('zh-CN') }}</span><span>{{ user.room }}</span><span><i />{{ user.status }}</span><span>{{ user.updatedAt }}</span></div></div></section>
      </template>

      <template v-else-if="activeSection === 'rooms'">
        <section class="admin-workspace"><header class="section-header"><div><h2>全部活跃房间</h2><span>{{ rooms.length }} 个演示房间</span></div></header><div class="admin-table"><div class="admin-row head"><span>房间</span><span>房主</span><span>人数</span><span>盲注</span><span>手牌</span><span>语音</span><span>状态</span></div><div v-for="room in rooms" :key="room.id" class="admin-row"><span><strong>{{ room.name }}</strong><small>{{ room.id }}</small></span><span>{{ room.owner }}</span><span>{{ room.players }}</span><span>{{ room.blind }}</span><span># {{ room.hand }}</span><span>{{ room.voice }}</span><span><i />{{ room.state }}</span></div></div></section>
      </template>

      <template v-else-if="activeSection === 'reports'">
        <section class="empty-operation"><AlertTriangle /><h2>2 条举报等待处理</h2><p>举报只包含房间、成员、时间与连接元数据，不包含语音录音。</p><button class="tool-button" type="button">打开处理队列</button></section>
      </template>

      <template v-else>
        <section class="admin-workspace"><header class="section-header"><div><h2>管理员操作</h2><span>积分重置和权限操作永久保留</span></div></header><div class="audit-list"><article v-for="audit in audits" :key="`${audit.time}-${audit.action}`"><time>{{ audit.time }}</time><span><strong>{{ audit.action }}</strong><small>{{ audit.operator }}</small></span><p>{{ audit.detail }}</p><b><CheckCircle2 />{{ audit.result }}</b></article></div></section>
      </template>
    </main>

    <div v-if="resetOpen" class="modal-backdrop" @click.self="closeReset">
      <section class="reset-dialog" role="dialog" aria-modal="true" aria-labelledby="reset-title">
        <header><span><AlertTriangle /></span><div><h2 id="reset-title">重置全站积分</h2><p>所有账号将立即进入新的积分周期。</p></div><button type="button" aria-label="关闭" @click="closeReset"><X /></button></header>
        <template v-if="!resetDone"><div class="reset-impact"><strong>不会中断活跃牌局</strong><p>当前余额统一变为 1,000；进行中牌局结束后，净输赢继续结算到新周期。</p></div><label>重置原因<textarea v-model="resetReason" rows="3" placeholder="至少填写 4 个字" /></label><label>输入 RESET ALL SCORES 确认<input v-model="confirmation" autocomplete="off" /></label><footer><button class="tool-button" type="button" @click="closeReset">取消</button><button class="danger-button" type="button" :disabled="confirmation !== 'RESET ALL SCORES' || resetReason.trim().length < 4" @click="performReset"><RotateCcw />确认重置</button></footer></template>
        <div v-else class="reset-success" role="status"><CheckCircle2 /><strong>重置已完成</strong><span>当前积分周期为 Epoch {{ epoch }}</span></div>
      </section>
    </div>
  </div>
</template>

