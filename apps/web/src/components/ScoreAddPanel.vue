<script setup lang="ts">
import { CircleAlert, Plus } from "@lucide/vue";
import { ref } from "vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const amount = ref<number | null>(500);
const error = ref("");
const success = ref("");
const busy = ref(false);

async function submit() {
	if (busy.value) return;
  error.value = "";
  success.value = "";
	busy.value = true;
  try {
    await store.addAccountPoints(amount.value ?? 0);
    success.value = `已增加 ${(amount.value ?? 0).toLocaleString("zh-CN")} 积分`;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : "增加积分失败";
	} finally {
		busy.value = false;
  }
}
</script>

<template>
  <form class="score-add-panel" @submit.prevent="submit">
    <div><label for="score-amount">增加局外积分</label><p>不会改变当前牌桌分</p></div>
    <div class="score-input-wrap"><input id="score-amount" v-model.number="amount" type="number" min="1" max="1000000000" step="1" inputmode="numeric" required :disabled="busy" /><button class="button primary" type="submit" :disabled="busy"><Plus />{{ busy ? "增加中" : "增加" }}</button></div>
    <p v-if="error" class="form-message error"><CircleAlert />{{ error }}</p>
    <p v-else-if="success" class="form-message success" role="status">{{ success }}</p>
  </form>
</template>

