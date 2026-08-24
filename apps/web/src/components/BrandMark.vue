<script setup lang="ts">
defineProps<{ compact?: boolean }>();
</script>

<template>
  <RouterLink class="brand-lockup" to="/" aria-label="Royal Flush 首页">
    <span class="brand-mark" aria-hidden="true">RF</span>
    <span v-if="!compact" class="brand-copy"><strong>Royal Flush</strong><small>好友德州牌局</small></span>
  </RouterLink>
</template>

