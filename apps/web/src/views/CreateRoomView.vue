<script setup lang="ts">
import type { BlindPreset, ChipDenomination, RoomConfig } from "@royal-flush/contracts";
import { BASE_CHIP_DENOMINATIONS, LARGE_CHIP_DENOMINATIONS } from "@royal-flush/contracts";
import { ArrowLeft, ArrowRight, Check, Headphones, LockKeyhole } from "@lucide/vue";
import { computed, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import AppHeader from "@/components/AppHeader.vue";
import { useGameStore } from "@/stores/game";

const router = useRouter();
const store = useGameStore();
const submitting = ref(false);
const submitError = ref("");
const form = reactive<{ name: string; maxPlayers: number; blindPreset: BlindPreset; actionSeconds: 20 | 30 | 45; voiceEnabled: boolean; largeChips: ChipDenomination[] }>({
  name: "", maxPlayers: 8, blindPreset: "5/10", actionSeconds: 30, voiceEnabled: true, largeChips: [],
});
const allowedChips = computed<ChipDenomination[]>(() => [...BASE_CHIP_DENOMINATIONS, ...form.largeChips]);

function toggleLargeChip(chip: ChipDenomination) {
  const index = form.largeChips.indexOf(chip);
  if (index >= 0) form.largeChips.splice(index, 1);
  else form.largeChips.push(chip);
  form.largeChips.sort((a, b) => a - b);
}

async function createRoom() {
  const config: RoomConfig = { name: form.name.trim(), maxPlayers: form.maxPlayers, blindPreset: form.blindPreset, actionSeconds: form.actionSeconds, voiceEnabled: form.voiceEnabled, chipDenominations: [...allowedChips.value] };
  submitting.value = true;
  submitError.value = "";
  try {
    const room = await store.createRoom(config);
    await router.push(`/rooms/${room.id}/waiting`);
  } catch (reason) {
    submitError.value = reason instanceof Error ? reason.message : "创建房间失败";
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="page-shell create-page">
    <AppHeader />
    <main class="form-page-content">
      <RouterLink class="back-link" to="/"><ArrowLeft />返回牌局大厅</RouterLink>
      <header class="form-page-heading"><div><h1>创建好友牌局</h1><p>房间创建后，盲注和筹码面额将锁定。</p></div><div class="creation-status"><LockKeyhole />仅通过邀请进入</div></header>
      <form class="room-form" @submit.prevent="createRoom">
        <section class="form-section"><header><h2>房间</h2><span>2–8 位好友</span></header><div class="field-grid"><label class="field wide"><span>房间名称</span><input v-model="form.name" required maxlength="24" placeholder="输入房间名称" /></label><label class="field"><span>最多人数</span><select v-model="form.maxPlayers"><option v-for="count in 7" :key="count" :value="count + 1">{{ count + 1 }} 人</option></select></label></div></section>
        <section class="form-section"><header><h2>牌局节奏</h2><span>创建后不可修改</span></header><fieldset><legend>盲注</legend><div class="segmented-control"><label v-for="preset in (['2/5', '5/10', '10/20'] as BlindPreset[])" :key="preset"><input v-model="form.blindPreset" type="radio" :value="preset" /><span>{{ preset }}</span></label></div></fieldset><fieldset><legend>行动时间</legend><div class="segmented-control"><label v-for="seconds in ([20, 30, 45] as const)" :key="seconds"><input v-model="form.actionSeconds" type="radio" :value="seconds" /><span>{{ seconds }} 秒</span></label></div></fieldset></section>
        <section class="form-section chip-config"><header><div><h2>筹码面额</h2><p>默认面额始终可重复组合</p></div><span><LockKeyhole />创建后锁定</span></header><div class="configured-chips"><span v-for="chip in BASE_CHIP_DENOMINATIONS" :key="chip" class="config-chip fixed">{{ chip }}<small>默认</small></span></div><fieldset><legend>额外大额筹码</legend><div class="large-chip-options"><label v-for="chip in LARGE_CHIP_DENOMINATIONS" :key="chip" :class="{ selected: form.largeChips.includes(chip) }"><input type="checkbox" :checked="form.largeChips.includes(chip)" @change="toggleLargeChip(chip)" /><span class="config-chip">{{ chip }}</span><strong>{{ form.largeChips.includes(chip) ? "已启用" : "不启用" }}</strong><Check v-if="form.largeChips.includes(chip)" /></label></div></fieldset><div class="chip-preview"><span>牌桌将提供</span><strong>{{ allowedChips.join(" / ") }}</strong></div></section>
        <section class="form-section voice-config"><header><h2>桌内语音</h2><Headphones /></header><label class="toggle-row"><span><strong>开启实时语音</strong><small>不录音、不保存、不转写</small></span><input v-model="form.voiceEnabled" type="checkbox" role="switch" /></label></section>
        <footer class="form-actions"><p>{{ submitError || "积分只用于娱乐计分，不具备货币价值。" }}</p><button class="button primary" type="submit" :disabled="submitting">{{ submitting ? "正在创建" : "创建并进入等候室" }}<ArrowRight /></button></footer>
      </form>
    </main>
  </div>
</template>
