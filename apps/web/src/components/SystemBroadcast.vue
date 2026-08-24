<script setup lang="ts">
import { Megaphone } from "@lucide/vue";

defineProps<{ text: string; at: string }>();
</script>

<template>
  <div class="system-broadcast" role="status" aria-live="polite">
    <strong><Megaphone />桌内广播</strong><span>{{ text }}</span><time>{{ at }}</time>
  </div>
</template>

