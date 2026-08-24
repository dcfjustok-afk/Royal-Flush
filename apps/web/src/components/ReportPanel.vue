<script setup lang="ts">
import { CheckCircle2, ChevronDown, Flag, LoaderCircle, Send } from "@lucide/vue";
import { computed, ref } from "vue";
import { api, apiMode, type ReportCategory } from "@/lib/api";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const open = ref(false);
const category = ref<ReportCategory>("conduct");
const subjectUserId = ref("");
const detail = ref("");
const busy = ref(false);
const error = ref("");
const success = ref("");
const requestId = ref(crypto.randomUUID());

const reportablePlayers = computed(() => store.snapshot.players.filter((player) => !player.isLocal));
const unavailableReason = computed(() => {
  if (!apiMode || !store.backendOnline) return "服务暂时不可用，请稍后重试";
  if (!store.snapshot.roomId) return "进入房间后才能提交举报";
  return "";
});
const valid = computed(() => detail.value.trim().length >= 2 && detail.value.length <= 1000 && !busy.value && !unavailableReason.value);

async function submit() {
  if (unavailableReason.value) {
    error.value = unavailableReason.value;
    return;
  }
  if (!valid.value) return;
  busy.value = true;
  error.value = "";
  success.value = "";
  try {
    await api.createReport({
      roomId: store.snapshot.roomId,
      subjectUserId: subjectUserId.value || undefined,
      category: category.value,
      detail: detail.value.trim(),
      requestId: requestId.value,
    });
    category.value = "conduct";
    subjectUserId.value = "";
    detail.value = "";
    requestId.value = crypto.randomUUID();
    success.value = "举报已登记，运营人员将按记录核查";
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "举报提交失败，请稍后重试";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="report-panel">
    <button class="report-panel-toggle" type="button" :aria-expanded="open" aria-controls="table-report-form" @click="open = !open">
      <span><Flag /><strong>举报问题</strong></span><ChevronDown :class="{ open }" />
    </button>
    <form v-if="open" id="table-report-form" @submit.prevent="submit">
      <label>问题类型<select v-model="category"><option value="conduct">行为秩序</option><option value="voice">语音问题</option><option value="technical">技术问题</option><option value="other">其他</option></select></label>
      <label>相关玩家<select v-model="subjectUserId"><option value="">整个房间 / 无特定玩家</option><option v-for="player in reportablePlayers" :key="player.id" :value="player.id">{{ player.name }}</option></select></label>
      <label>问题说明<textarea v-model="detail" minlength="2" maxlength="1000" rows="4" placeholder="请填写可供核查的具体情况" required /></label>
      <div class="report-form-footer"><span>{{ detail.length }} / 1000</span><button class="tool-button" type="submit" :disabled="!valid"><LoaderCircle v-if="busy" class="spin" /><Send v-else />{{ busy ? "正在提交" : "提交举报" }}</button></div>
      <p v-if="error || unavailableReason" class="report-feedback error" role="alert">{{ error || unavailableReason }}</p>
      <p v-else-if="success" class="report-feedback success" role="status"><CheckCircle2 />{{ success }}</p>
    </form>
  </section>
</template>
