<script setup lang="ts">
import type { PlayerSnapshot } from "@royal-flush/contracts";
import { MicOff, Radio, UserPlus, WifiOff } from "@lucide/vue";
import { computed, type CSSProperties } from "vue";
import VoiceMeter from "./VoiceMeter.vue";

const props = withDefaults(defineProps<{ player?: PlayerSnapshot; seat: number; turnProgress?: number; voiceConnected?: boolean }>(), {
  turnProgress: 0,
  voiceConnected: false,
});
const turnStyle = computed(() => ({
  "--turn-progress": `${Math.max(0, Math.min(1, props.turnProgress))}turn`,
}) as CSSProperties);
</script>

<template>
  <article class="player-seat" :class="[`seat-${seat}`, { speaking: voiceConnected && player?.isSpeaking, acting: player?.isCurrentActor, local: player?.isLocal, disconnected: player?.status === 'disconnected' }]" :style="turnStyle">
    <template v-if="player">
      <header>
        <span class="seat-avatar">{{ player.initials }}</span>
        <span class="seat-name">{{ player.name }}<b v-if="player.isDealer" class="dealer-button">D</b></span>
        <VoiceMeter v-if="voiceConnected && player.isSpeaking" active :label="`${player.name}正在说话`" />
        <WifiOff v-else-if="player.status === 'disconnected'" class="seat-state-icon" aria-label="已断线" />
        <MicOff v-else-if="player.isMuted" class="seat-state-icon" aria-label="已禁言" />
        <MicOff v-else-if="player.status === 'away'" class="seat-state-icon" aria-label="已暂离" />
        <Radio v-else class="seat-state-icon" aria-hidden="true" />
      </header>
      <div class="seat-values"><span><span class="score-label-full">牌桌分</span><span class="score-label-short">牌桌</span><strong>{{ player.tablePoints.toLocaleString("zh-CN") }}</strong></span><span><span class="score-label-full">局外积分</span><span class="score-label-short">局外</span><strong>{{ player.accountPoints.toLocaleString("zh-CN") }}</strong></span></div>
      <span v-if="player.isCurrentActor" class="turn-label">行动中</span>
    </template>
    <template v-else>
      <UserPlus class="empty-seat-icon" aria-hidden="true" />
      <span class="empty-seat-copy">空位</span>
    </template>
  </article>
</template>
