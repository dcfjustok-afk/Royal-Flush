<script setup lang="ts">
import {
  Activity, AlertTriangle, Ban, CheckCircle2, CircleUserRound, Clock3,
  DoorOpen, FileClock, Gauge, Inbox, KeyRound, LoaderCircle, Radio, RefreshCw, RotateCcw,
  Search, Send, ShieldAlert, ShieldCheck, Unlock, Users, WifiOff, X, XCircle,
} from "@lucide/vue";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  adminApi, ApiError, apiMode,
  type AdminAudit, type AdminRoom, type AdminRoomSnapshot, type OperationsUser, type Report, type SessionUser,
} from "./api";

type Section = "overview" | "users" | "rooms" | "reports" | "audit";

const now = new Date();
const activeSection = ref<Section>("overview");
const authState = ref<"checking" | "anonymous" | "authorized" | "forbidden">("checking");
const operator = ref<SessionUser | null>(null);
const loginAccount = ref("");
const loginPassword = ref("");
const authBusy = ref(false);
const authError = ref("");
const search = ref("");
const loading = ref(false);
const loadErrors = ref<string[]>([]);
const epoch = ref<number | null>(null);
const rooms = ref<AdminRoom[]>([]);
const users = ref<OperationsUser[]>([]);
const reports = ref<Report[]>([]);
const audits = ref<AdminAudit[]>([]);

const resetOpen = ref(false);
const resetReason = ref("");
const confirmation = ref("");
const resetDone = ref(false);
const resetBusy = ref(false);
const resetError = ref("");
const moderationTarget = ref<OperationsUser | null>(null);
const moderationReason = ref("");
const moderationBusy = ref(false);
const moderationError = ref("");
const reportTarget = ref<Report | null>(null);
const reportDecision = ref<"resolved" | "dismissed">("resolved");
const reportReason = ref("");
const reportBusy = ref(false);
const reportError = ref("");
const roomDetail = ref<AdminRoomSnapshot | null>(null);
const roomDetailBusy = ref(false);
const roomDetailError = ref("");
let resetTimer: number | undefined;
let searchTimer: number | undefined;
let userRequestGeneration = 0;
let roomDetailRequestGeneration = 0;

const roomById = computed(() => new Map(rooms.value.map((room) => [room.id, room])));
const filteredUsers = computed(() => {
  const query = search.value.trim().toLowerCase();
  if (!query || apiMode) return users.value;
  return users.value.filter((user) => `${user.id}${user.nickname}${user.phone}${user.activeRoomId ?? ""}`.toLowerCase().includes(query));
});
const openReports = computed(() => reports.value.filter((report) => report.status === "open" || report.status === "reviewing"));
const playingRooms = computed(() => rooms.value.filter((room) => room.status === "playing").length);
const onlinePlayers = computed(() => rooms.value.reduce((total, room) => total + room.onlinePlayers, 0));
const credentialsValid = computed(() => loginAccount.value.trim().length > 0 && loginPassword.value.length > 0);

function errorMessage(reason: unknown, fallback: string) {
  return reason instanceof Error ? reason.message : fallback;
}

async function loadAll() {
  if (!apiMode || authState.value !== "authorized") return;
  loading.value = true;
  loadErrors.value = [];
  const userGeneration = ++userRequestGeneration;
  const userQuery = search.value.trim();
  const [epochResult, roomResult, userResult, reportResult, auditResult] = await Promise.allSettled([
    adminApi.epochs(), adminApi.rooms(), adminApi.users(userQuery), adminApi.reports(), adminApi.audits(),
  ]);
  if (epochResult.status === "fulfilled" && epochResult.value.epochs?.[0]) epoch.value = epochResult.value.epochs[0].id;
  else if (epochResult.status === "rejected") loadErrors.value.push(`积分周期：${errorMessage(epochResult.reason, "加载失败")}`);
  if (roomResult.status === "fulfilled") rooms.value = roomResult.value.rooms ?? [];
  else loadErrors.value.push(`活跃房间：${errorMessage(roomResult.reason, "加载失败")}`);
  if (userGeneration === userRequestGeneration) {
    if (userResult.status === "fulfilled") users.value = userResult.value.users ?? [];
    else loadErrors.value.push(`用户列表：${errorMessage(userResult.reason, "加载失败")}`);
  }
  if (reportResult.status === "fulfilled") reports.value = reportResult.value.reports ?? [];
  else loadErrors.value.push(`举报队列：${errorMessage(reportResult.reason, "加载失败")}`);
  if (auditResult.status === "fulfilled") audits.value = auditResult.value.audits ?? [];
  else loadErrors.value.push(`审计记录：${errorMessage(auditResult.reason, "加载失败")}`);
  loading.value = false;
}

function acceptOperator(user: SessionUser) {
  operator.value = user;
  const authorized = Boolean(user.permissions["admin:read"]);
  authState.value = authorized ? "authorized" : "forbidden";
  return authorized;
}

async function initialize() {
  if (!apiMode) {
    authState.value = "anonymous";
    authError.value = "运营服务未配置，无法读取真实数据";
    return;
  }
  authState.value = "checking";
  authError.value = "";
  try {
    if (acceptOperator((await adminApi.me()).user)) await loadAll();
  } catch (reason) {
    if (reason instanceof ApiError && reason.status === 401) authState.value = "anonymous";
    else {
      authState.value = "anonymous";
      authError.value = errorMessage(reason, "运营身份暂时无法验证，请重试");
    }
  }
}

async function loginAdmin() {
  if (!credentialsValid.value || authBusy.value) return;
  authBusy.value = true;
  authError.value = "";
  try {
    await adminApi.login(loginAccount.value.trim(), loginPassword.value);
    if (acceptOperator((await adminApi.me()).user)) await loadAll();
  } catch (reason) {
    authError.value = errorMessage(reason, "登录失败，请检查账号或密码后重试");
  } finally {
    authBusy.value = false;
  }
}

function useAnotherAccount() {
  authState.value = "anonymous";
  operator.value = null;
  loginAccount.value = "";
  loginPassword.value = "";
  authError.value = "";
}

async function refreshSection() {
  if (!apiMode) return;
  loading.value = true;
  loadErrors.value = [];
  try {
    if (activeSection.value === "users") {
      const generation = ++userRequestGeneration;
      const result = await adminApi.users(search.value.trim());
      if (generation === userRequestGeneration) users.value = result.users ?? [];
    }
    else if (activeSection.value === "reports") reports.value = (await adminApi.reports()).reports ?? [];
    else if (activeSection.value === "audit") audits.value = (await adminApi.audits()).audits ?? [];
    else rooms.value = (await adminApi.rooms()).rooms ?? [];
  } catch (reason) {
    loadErrors.value = [errorMessage(reason, "刷新失败，请稍后重试")];
  } finally {
    loading.value = false;
  }
}

async function performReset() {
  if (confirmation.value !== "RESET ALL SCORES" || resetReason.value.trim().length < 4 || resetBusy.value) return;
  resetBusy.value = true;
  resetError.value = "";
  try {
    const previousEpoch = epoch.value;
    if (!apiMode) throw new Error("运营服务未配置");
    epoch.value = (await adminApi.resetScores(resetReason.value.trim())).epoch;
    audits.value.unshift({ id: crypto.randomUUID(), administratorId: operator.value?.id ?? "unknown", action: "score.reset_all", targetType: "score_epoch", targetId: String(epoch.value), reason: resetReason.value.trim(), requestId: crypto.randomUUID(), metadata: { previousEpoch }, createdAt: new Date().toISOString() });
    resetDone.value = true;
    resetTimer = window.setTimeout(closeReset, 1400);
  } catch (reason) {
    resetError.value = errorMessage(reason, "积分重置失败，请检查权限后重试");
  } finally {
    resetBusy.value = false;
  }
}

function closeReset() {
  resetOpen.value = false;
  resetDone.value = false;
  resetReason.value = "";
  confirmation.value = "";
  resetError.value = "";
}

function openModeration(user: OperationsUser) {
  moderationTarget.value = user;
  moderationReason.value = "";
  moderationError.value = "";
}

async function applyModeration() {
  const target = moderationTarget.value;
  if (!target || moderationReason.value.trim().length < 2 || moderationBusy.value) return;
  moderationBusy.value = true;
  moderationError.value = "";
  try {
    if (!apiMode) throw new Error("运营服务未配置，无法修改用户状态");
    const updated = (await adminApi.setUserBanned(target.id, !target.banned, moderationReason.value.trim())).user;
    users.value = users.value.map((user) => user.id === updated.id ? updated : user);
    moderationTarget.value = null;
  } catch (reason) {
    moderationError.value = errorMessage(reason, "用户状态修改失败，请重试");
  } finally {
    moderationBusy.value = false;
  }
}

function openReport(report: Report) {
  reportTarget.value = report;
  reportDecision.value = "resolved";
  reportReason.value = "";
  reportError.value = "";
}

async function handleReport() {
  const target = reportTarget.value;
  if (!target || reportReason.value.trim().length < 2 || reportBusy.value) return;
  reportBusy.value = true;
  reportError.value = "";
  try {
    if (!apiMode) throw new Error("运营服务未配置，无法处理举报");
    const updated = (await adminApi.resolveReport(target.id, reportDecision.value, reportReason.value.trim())).report;
    reports.value = reports.value.map((report) => report.id === updated.id ? updated : report);
    reportTarget.value = null;
  } catch (reason) {
    reportError.value = errorMessage(reason, "举报处理失败，请重试");
  } finally {
    reportBusy.value = false;
  }
}

async function inspectRoom(room: AdminRoom) {
	const generation = ++roomDetailRequestGeneration;
  roomDetail.value = null;
  roomDetailError.value = "";
  roomDetailBusy.value = true;
  try {
    if (!apiMode) throw new Error("运营服务未配置，无法读取房间详情");
    const detail = await adminApi.room(room.id);
		if (generation !== roomDetailRequestGeneration) return;
		roomDetail.value = detail;
  } catch (reason) {
		if (generation === roomDetailRequestGeneration) roomDetailError.value = errorMessage(reason, "房间详情加载失败");
  } finally {
		if (generation === roomDetailRequestGeneration) roomDetailBusy.value = false;
  }
}

function closeRoomDetail() {
	roomDetailRequestGeneration++;
	roomDetail.value = null;
	roomDetailError.value = "";
	roomDetailBusy.value = false;
}

function maskPhone(phone: string) {
  return /^1\d{10}$/.test(phone) ? `${phone.slice(0, 3)} **** ${phone.slice(-4)}` : phone || "未绑定";
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short", hour12: false, timeZone: "Asia/Shanghai" }).format(new Date(value));
}

function roomLabel(roomId?: string) {
  return roomId ? roomById.value.get(roomId)?.code ?? roomId : "未在房间";
}

function statusLabel(status: AdminRoom["status"]) {
  return status === "playing" ? "进行中" : status === "waiting" ? "等候中" : "已结束";
}

function reportCategory(category: Report["category"]) {
  return { conduct: "行为秩序", voice: "语音问题", technical: "技术问题", other: "其他" }[category];
}

function auditAction(action: string) {
  return { "score.reset_all": "全站积分重置", "user.ban": "用户封禁", "user.unban": "解除封禁", "report.resolved": "举报已解决", "report.dismissed": "举报已驳回" }[action] ?? action;
}

watch(search, () => {
  if (!apiMode) return;
  window.clearTimeout(searchTimer);
	const generation = ++userRequestGeneration;
	const query = search.value.trim();
  searchTimer = window.setTimeout(async () => {
    try {
			const result = await adminApi.users(query);
			if (generation === userRequestGeneration) users.value = result.users ?? [];
		} catch (reason) {
			if (generation === userRequestGeneration) loadErrors.value = [errorMessage(reason, "用户搜索失败")];
		}
  }, 300);
});

onMounted(initialize);
onBeforeUnmount(() => {
	userRequestGeneration++;
	roomDetailRequestGeneration++;
  window.clearTimeout(resetTimer);
  window.clearTimeout(searchTimer);
});
</script>

<template>
  <div v-if="apiMode && authState !== 'authorized'" class="admin-auth-shell">
    <header class="admin-auth-header"><a class="admin-brand" href="/"><span>RF</span><strong>Royal Flush<small>运营控制台</small></strong></a><span>仅限授权运营人员</span></header>
    <main class="admin-auth-main">
      <section v-if="authState === 'checking'" class="auth-state" aria-live="polite"><LoaderCircle class="spin" /><h1>正在验证运营身份</h1><p>正在读取当前会话与权限。</p></section>
      <section v-else-if="authState === 'forbidden'" class="auth-state forbidden" role="alert"><ShieldAlert /><h1>当前账号没有运营权限</h1><p>{{ operator ? maskPhone(operator.phone) : '该账号' }} 已完成登录，但不在授权运营名单中。</p><button class="tool-button" type="button" @click="useAnotherAccount"><KeyRound />更换管理员账号</button></section>
      <form v-else class="admin-login" @submit.prevent="loginAdmin">
        <div class="auth-title"><ShieldCheck /><div><h1>运营身份验证</h1><p>使用平台管理员账号和密码进入控制台。</p></div></div>
        <label>管理员账号<input v-model.trim="loginAccount" inputmode="numeric" autocomplete="username" placeholder="请输入管理员账号" :disabled="authBusy" /></label>
        <label>管理员密码<input v-model="loginPassword" type="password" autocomplete="current-password" placeholder="请输入管理员密码" :disabled="authBusy" /></label>
        <p v-if="authError" class="auth-error" role="alert">{{ authError }}</p>
        <footer>
          <button class="auth-submit" type="submit" :disabled="authBusy || !credentialsValid"><LoaderCircle v-if="authBusy" class="spin" /><Send v-else />{{ authBusy ? '请稍候' : '登录运营台' }}</button>
        </footer>
      </form>
    </main>
    <footer class="admin-auth-footer"><span>积分仅用于娱乐计分，不具备货币价值</span><span>操作将写入永久审计记录</span></footer>
  </div>

  <div v-else class="admin-shell">
    <aside class="admin-sidebar">
      <a class="admin-brand" href="/"><span>RF</span><strong>Royal Flush<small>运营控制台</small></strong></a>
      <nav aria-label="运营导航">
        <button :class="{ active: activeSection === 'overview' }" @click="activeSection = 'overview'"><Gauge />运行概览</button>
        <button :class="{ active: activeSection === 'users' }" @click="activeSection = 'users'"><Users />用户与积分</button>
        <button :class="{ active: activeSection === 'rooms' }" @click="activeSection = 'rooms'"><DoorOpen />活跃房间</button>
        <button :class="{ active: activeSection === 'reports' }" @click="activeSection = 'reports'"><AlertTriangle />举报处理<span v-if="openReports.length" class="nav-count">{{ openReports.length }}</span></button>
        <button :class="{ active: activeSection === 'audit' }" @click="activeSection = 'audit'"><FileClock />审计记录</button>
      </nav>
      <div class="admin-permission"><ShieldCheck /><span><strong>平台运营管理员</strong><small>admin:read · report:manage</small></span></div>
    </aside>

    <main class="admin-main">
      <header class="admin-topbar">
        <div><h1>{{ { overview: '运行概览', users: '用户与积分', rooms: '活跃房间', reports: '举报处理', audit: '审计记录' }[activeSection] }}</h1><span>{{ new Intl.DateTimeFormat('zh-CN', { dateStyle: 'long', timeZone: 'Asia/Shanghai' }).format(now) }} · Asia/Shanghai</span></div>
        <div class="operator"><span class="service-live">{{ apiMode ? '实时接口' : '服务未配置' }}</span><div class="operator-identity"><CircleUserRound /><span><strong>{{ operator?.nickname ?? '运营员' }}</strong><small>{{ operator?.phone ? maskPhone(operator.phone) : '--' }}</small></span></div></div>
      </header>

      <div v-if="loadErrors.length" class="error-band" role="alert"><WifiOff /><span><strong>部分数据未更新</strong>{{ loadErrors.join('；') }}</span><button class="tool-button" type="button" @click="loadAll"><RefreshCw />重试</button></div>

      <template v-if="activeSection === 'overview'">
        <section class="summary-band" aria-label="运行摘要">
          <div><span>在线牌友</span><strong>{{ onlinePlayers }}</strong><small><Activity />按活跃房间实时汇总</small></div>
          <div><span>活跃房间</span><strong>{{ rooms.length }}</strong><small><Radio />{{ playingRooms }} 桌正在出牌</small></div>
          <div><span>已登记用户</span><strong>{{ users.length }}</strong><small><Users />当前查询范围</small></div>
          <div><span>待处理举报</span><strong>{{ openReports.length }}</strong><small :class="{ warning: openReports.length > 0 }">{{ openReports.length ? '需要运营复核' : '队列已清空' }}</small></div>
        </section>
        <section class="operations-grid">
          <div class="operations-main">
            <header class="section-header"><div><h2>活跃房间</h2><span>按创建时间排序</span></div><button class="tool-button" type="button" :disabled="loading" @click="refreshSection"><RefreshCw :class="{ spin: loading }" />刷新</button></header>
            <div class="admin-table"><div class="admin-row head"><span>房间</span><span>房主</span><span>人数</span><span>盲注</span><span>手牌</span><span>语音</span><span>状态</span></div><button v-for="room in rooms.slice(0, 8)" :key="room.id" class="admin-row row-button" type="button" @click="inspectRoom(room)"><span><strong>{{ room.name }}</strong><small>{{ room.code }}</small></span><span>{{ room.ownerName }}</span><span>{{ room.onlinePlayers }} / {{ room.maxPlayers }}</span><span>{{ room.blindPreset }}</span><span># {{ String(room.handNumber).padStart(3, '0') }}</span><span>{{ room.voiceEnabled ? '已启用' : '已关闭' }}</span><span><i :class="{ waiting: room.status === 'waiting' }" />{{ statusLabel(room.status) }}</span></button><div v-if="!rooms.length" class="table-empty"><Inbox />当前没有活跃房间</div></div>
          </div>
          <aside class="operations-side">
            <section class="score-epoch"><header><h2>积分周期</h2><RotateCcw /></header><span>当前 Epoch</span><strong>{{ epoch ?? '--' }}</strong><p>全站基线 1,000，活跃牌局结束后照常结算净输赢。</p><button class="danger-button" type="button" @click="resetOpen = true"><RotateCcw />重置全站积分</button></section>
            <section class="system-feed"><header><h2>最近审计</h2><Clock3 /></header><ol><li v-for="audit in audits.slice(0, 4)" :key="audit.id"><time>{{ formatTime(audit.createdAt).split(' ').at(-1) }}</time><span>{{ auditAction(audit.action) }} · {{ audit.reason }}</span></li><li v-if="!audits.length"><span>暂无管理员操作</span></li></ol></section>
          </aside>
        </section>
      </template>

      <template v-else-if="activeSection === 'users'">
        <section class="admin-workspace"><header class="workspace-tools"><label><Search /><input v-model="search" aria-label="搜索用户" placeholder="搜索昵称、手机号或用户 ID" /></label><button class="danger-button" type="button" @click="resetOpen = true"><RotateCcw />重置全站积分</button></header>
          <div class="admin-table users-table"><div class="admin-row head"><span>用户</span><span>手机号</span><span>局外积分</span><span>当前房间</span><span>状态</span><span>最后事件</span><span>操作</span></div><div v-for="user in filteredUsers" :key="user.id" class="admin-row"><span class="truncate"><strong>{{ user.nickname }}</strong><small>{{ user.id }}</small></span><span>{{ maskPhone(user.phone) }}</span><span :class="{ negative: user.balance < 0 }">{{ user.balance.toLocaleString('zh-CN') }}</span><span class="truncate">{{ roomLabel(user.activeRoomId) }}</span><span><i :class="{ banned: user.banned }" />{{ user.banned ? '已封禁' : '正常' }}</span><span>{{ formatTime(user.updatedAt) }}</span><span><button class="icon-action" type="button" :title="user.banned ? '解除封禁' : '封禁用户'" @click="openModeration(user)"><Unlock v-if="user.banned" /><Ban v-else /></button></span></div><div v-if="!filteredUsers.length" class="table-empty"><Search />没有匹配的用户</div></div>
        </section>
      </template>

      <template v-else-if="activeSection === 'rooms'">
        <section class="admin-workspace"><header class="section-header"><div><h2>全部活跃房间</h2><span>{{ rooms.length }} 个房间由当前服务实例持有租约</span></div><button class="tool-button" type="button" :disabled="loading" @click="refreshSection"><RefreshCw :class="{ spin: loading }" />刷新</button></header><div class="admin-table"><div class="admin-row head"><span>房间</span><span>房主</span><span>在线 / 座位</span><span>盲注</span><span>手牌</span><span>语音</span><span>状态</span></div><button v-for="room in rooms" :key="room.id" class="admin-row row-button" type="button" @click="inspectRoom(room)"><span><strong>{{ room.name }}</strong><small>{{ room.code }} · v{{ room.version }}</small></span><span>{{ room.ownerName }}</span><span>{{ room.onlinePlayers }} / {{ room.players }}</span><span>{{ room.blindPreset }}</span><span># {{ room.handNumber }}</span><span>{{ room.voiceEnabled ? '已启用' : '已关闭' }}</span><span><i :class="{ waiting: room.status === 'waiting' }" />{{ statusLabel(room.status) }}</span></button><div v-if="!rooms.length" class="table-empty"><DoorOpen />当前没有活跃房间</div></div></section>
      </template>

      <template v-else-if="activeSection === 'reports'">
        <section class="admin-workspace"><header class="section-header"><div><h2>举报处理队列</h2><span>{{ openReports.length }} 条等待复核，语音不会被录制或附在举报中</span></div><button class="tool-button" type="button" :disabled="loading" @click="refreshSection"><RefreshCw :class="{ spin: loading }" />刷新</button></header><div class="report-list"><article v-for="report in reports" :key="report.id"><header><span class="report-category">{{ reportCategory(report.category) }}</span><time>{{ formatTime(report.createdAt) }}</time><b :class="report.status">{{ { open: '待处理', reviewing: '复核中', resolved: '已解决', dismissed: '已驳回' }[report.status] }}</b></header><p>{{ report.detail }}</p><footer><span>举报人 {{ report.reporterId }}<template v-if="report.roomId"> · 房间 {{ roomLabel(report.roomId) }}</template><template v-if="report.subjectUserId"> · 对象 {{ report.subjectUserId }}</template></span><button v-if="report.status === 'open' || report.status === 'reviewing'" class="tool-button" type="button" @click="openReport(report)"><CheckCircle2 />处理</button></footer></article><div v-if="!reports.length" class="table-empty"><Inbox />举报队列为空</div></div></section>
      </template>

      <template v-else>
        <section class="admin-workspace"><header class="section-header"><div><h2>管理员操作</h2><span>积分重置、封禁和举报处理永久保留</span></div><button class="tool-button" type="button" :disabled="loading" @click="refreshSection"><RefreshCw :class="{ spin: loading }" />刷新</button></header><div class="audit-list"><article v-for="audit in audits" :key="audit.id"><time>{{ formatTime(audit.createdAt) }}</time><span><strong>{{ auditAction(audit.action) }}</strong><small>{{ audit.administratorId }}</small></span><p>{{ audit.reason }}<small v-if="audit.targetId">{{ audit.targetType }} · {{ audit.targetId }}</small></p><b><CheckCircle2 />已记录</b></article><div v-if="!audits.length" class="table-empty"><FileClock />暂无审计记录</div></div></section>
      </template>
    </main>

    <aside v-if="roomDetailBusy || roomDetail || roomDetailError" class="detail-drawer" aria-label="房间详情"><header><div><h2>{{ roomDetail?.roomName ?? '房间详情' }}</h2><span>{{ roomDetail?.roomCode }}</span></div><button type="button" aria-label="关闭房间详情" @click="closeRoomDetail"><X /></button></header><div v-if="roomDetailBusy" class="drawer-state"><LoaderCircle class="spin" />正在读取权威快照</div><div v-else-if="roomDetailError" class="drawer-state error"><WifiOff />{{ roomDetailError }}</div><template v-else-if="roomDetail"><dl><div><dt>牌局阶段</dt><dd>{{ roomDetail.street }}</dd></div><div><dt>手牌 / 版本</dt><dd>#{{ roomDetail.handNumber }} · v{{ roomDetail.version }}</dd></div><div><dt>底池</dt><dd>{{ roomDetail.pot.toLocaleString('zh-CN') }}</dd></div><div><dt>盲注</dt><dd>{{ roomDetail.config.blindPreset }}</dd></div></dl><h3>座位状态</h3><ol class="drawer-players"><li v-for="player in roomDetail.players" :key="player.id"><span><strong>{{ player.name }}</strong><small>{{ player.id }} · {{ player.seat + 1 }} 号位</small></span><b>{{ player.tablePoints.toLocaleString('zh-CN') }}</b><em>{{ player.status }}</em></li></ol><h3>筹码面额</h3><div class="drawer-chips"><span v-for="chip in roomDetail.config.chipDenominations" :key="chip">{{ chip }}</span></div></template></aside>

    <div v-if="resetOpen" class="modal-backdrop" @click.self="closeReset"><section class="reset-dialog" role="dialog" aria-modal="true" aria-labelledby="reset-title"><header><span><AlertTriangle /></span><div><h2 id="reset-title">重置全站积分</h2><p>所有账号将立即进入新的积分周期。</p></div><button type="button" aria-label="关闭" @click="closeReset"><X /></button></header><template v-if="!resetDone"><div class="reset-impact"><strong>不会中断活跃牌局</strong><p>当前余额统一变为 1,000；进行中牌局结束后，净输赢继续结算到新周期。</p></div><label>重置原因<textarea v-model="resetReason" maxlength="500" rows="3" placeholder="至少填写 4 个字" /></label><label>输入 RESET ALL SCORES 确认<input v-model="confirmation" autocomplete="off" /></label><p v-if="resetError" class="reset-error" role="alert">{{ resetError }}</p><footer><button class="tool-button" type="button" @click="closeReset">取消</button><button class="danger-button" type="button" :disabled="resetBusy || confirmation !== 'RESET ALL SCORES' || resetReason.trim().length < 4" @click="performReset"><LoaderCircle v-if="resetBusy" class="spin" /><RotateCcw v-else />{{ resetBusy ? '正在重置' : '确认重置' }}</button></footer></template><div v-else class="reset-success" role="status"><CheckCircle2 /><strong>重置已完成</strong><span>当前积分周期为 Epoch {{ epoch }}</span></div></section></div>

    <div v-if="moderationTarget" class="modal-backdrop" @click.self="moderationTarget = null"><section class="action-dialog" role="dialog" aria-modal="true" aria-labelledby="moderation-title"><header><span :class="{ safe: moderationTarget.banned }"><Unlock v-if="moderationTarget.banned" /><Ban v-else /></span><div><h2 id="moderation-title">{{ moderationTarget.banned ? '解除用户封禁' : '封禁用户' }}</h2><p>{{ moderationTarget.nickname }} · {{ moderationTarget.id }}</p></div><button type="button" aria-label="关闭" @click="moderationTarget = null"><X /></button></header><label>操作原因<textarea v-model="moderationReason" maxlength="500" rows="3" placeholder="至少填写 2 个字" /></label><p v-if="moderationError" class="reset-error" role="alert">{{ moderationError }}</p><footer><button class="tool-button" type="button" @click="moderationTarget = null">取消</button><button :class="moderationTarget.banned ? 'confirm-button' : 'danger-button'" type="button" :disabled="moderationBusy || moderationReason.trim().length < 2" @click="applyModeration"><LoaderCircle v-if="moderationBusy" class="spin" /><Unlock v-else-if="moderationTarget.banned" /><Ban v-else />确认{{ moderationTarget.banned ? '解封' : '封禁' }}</button></footer></section></div>

    <div v-if="reportTarget" class="modal-backdrop" @click.self="reportTarget = null"><section class="action-dialog" role="dialog" aria-modal="true" aria-labelledby="report-title"><header><span><AlertTriangle /></span><div><h2 id="report-title">处理举报</h2><p>{{ reportCategory(reportTarget.category) }} · {{ reportTarget.id }}</p></div><button type="button" aria-label="关闭" @click="reportTarget = null"><X /></button></header><div class="decision-control" role="group" aria-label="处理结论"><button type="button" :class="{ active: reportDecision === 'resolved' }" @click="reportDecision = 'resolved'"><CheckCircle2 />已解决</button><button type="button" :class="{ active: reportDecision === 'dismissed' }" @click="reportDecision = 'dismissed'"><XCircle />驳回</button></div><label>处理原因<textarea v-model="reportReason" maxlength="500" rows="3" placeholder="至少填写 2 个字" /></label><p v-if="reportError" class="reset-error" role="alert">{{ reportError }}</p><footer><button class="tool-button" type="button" @click="reportTarget = null">取消</button><button class="confirm-button" type="button" :disabled="reportBusy || reportReason.trim().length < 2" @click="handleReport"><LoaderCircle v-if="reportBusy" class="spin" /><CheckCircle2 v-else />提交处理结果</button></footer></section></div>
  </div>
</template>
