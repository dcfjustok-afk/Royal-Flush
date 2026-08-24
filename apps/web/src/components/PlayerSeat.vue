<script setup lang="ts">
import type { PlayerSnapshot } from "@royal-flush/contracts";
import { MicOff, Radio, UserPlus, WifiOff } from "@lucide/vue";
import VoiceMeter from "./VoiceMeter.vue";

defineProps<{ player?: PlayerSnapshot; seat: number }>();
</script>

<template>
  <article class="player-seat" :class="[`seat-${seat}`, { speaking: player?.isSpeaking, acting: player?.isCurrentActor, local: player?.isLocal, disconnected: player?.status === 'disconnected' }]">
    <template v-if="player">
      <header>
        <span class="seat-avatar">{{ player.initials }}</span>
        <span class="seat-name">{{ player.name }}<b v-if="player.isDealer" class="dealer-button">D</b></span>
        <VoiceMeter v-if="player.isSpeaking" active :label="`${player.name}正在说话`" />
        <WifiOff v-else-if="player.status === 'disconnected'" class="seat-state-icon" aria-label="已断线" />
        <MicOff v-else-if="player.isMuted" class="seat-state-icon" aria-label="已禁言" />
        <MicOff v-else-if="player.status === 'away'" class="seat-state-icon" aria-label="已暂离" />
        <Radio v-else class="seat-state-icon" aria-hidden="true" />
      </header>
      <div class="seat-values"><span>牌桌分<strong>{{ player.tablePoints.toLocaleString("zh-CN") }}</strong></span><span>局外积分<strong>{{ player.accountPoints.toLocaleString("zh-CN") }}</strong></span></div>
      <span v-if="player.isCurrentActor" class="turn-label">行动中</span>
    </template>
    <template v-else>
      <UserPlus class="empty-seat-icon" aria-hidden="true" />
      <span class="empty-seat-copy">空位</span>
    </template>
  </article>
</template>
