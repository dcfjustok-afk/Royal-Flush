<script setup lang="ts">
import { computed, onMounted } from "vue";
import { RouterView, useRoute } from "vue-router";
import LegalDisclaimer from "@/components/LegalDisclaimer.vue";
import { useGameStore } from "@/stores/game";

const route = useRoute();
const store = useGameStore();
const immersive = computed(() => route.meta.immersive === true);

onMounted(() => store.probeBackend());
</script>

<template>
  <RouterView />
  <LegalDisclaimer v-if="!immersive" />
</template>

