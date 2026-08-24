<script setup lang="ts">
import { ref } from "vue";
import { MessageCircleMore } from "@lucide/vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
const sending = ref("");
const feedback = ref("");
const quickMessages = ["好牌", "稍等一下", "打得漂亮", "下一手"] as const;

async function send(message: typeof quickMessages[number]) {
  if (sending.value) return;
  sending.value = message;
  feedback.value = "";
  try {
    await store.sendCommand("room.quick_message", { message });
    feedback.value = `已发送：${message}`;
  } catch (reason) {
    feedback.value = reason instanceof Error ? reason.message : "快捷消息发送失败";
  } finally {
    sending.value = "";
  }
}
</script>

<template>
  <section class="quick-message-panel">
    <header><h3>快捷消息</h3><MessageCircleMore /></header>
    <div><button v-for="message in quickMessages" :key="message" type="button" :disabled="Boolean(sending)" @click="send(message)">{{ message }}</button></div>
    <p v-if="feedback" role="status">{{ feedback }}</p>
  </section>
</template>
