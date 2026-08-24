<script setup lang="ts">
import type { PlayingCard } from "@royal-flush/contracts";
import { computed } from "vue";

const props = defineProps<{ card: PlayingCard; compact?: boolean }>();
const suitMap = {
  spades: { glyph: "♠", name: "黑桃", red: false },
  hearts: { glyph: "♥", name: "红桃", red: true },
  diamonds: { glyph: "♦", name: "方片", red: true },
  clubs: { glyph: "♣", name: "梅花", red: false },
} as const;
const suit = computed(() => suitMap[props.card.suit]);
</script>

<template>
  <span class="playing-card" :class="{ red: suit.red, compact }" :aria-label="`${suit.name}${card.rank}`">
    <strong>{{ card.rank }}</strong><span aria-hidden="true">{{ suit.glyph }}</span>
  </span>
</template>

