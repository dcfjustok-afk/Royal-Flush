<script setup lang="ts">
withDefaults(defineProps<{ active?: boolean; label?: string }>(), { active: false, label: "语音电平" });
</script>

<template>
  <span class="voice-meter" :class="{ active }" :aria-label="label" role="img">
    <i v-for="bar in 5" :key="bar" :style="{ '--bar': bar }" />
  </span>
</template>

