<script setup lang="ts">
import { Bell, CircleUserRound, History, Plus, Radio, Wifi } from "@lucide/vue";
import BrandMark from "./BrandMark.vue";
import VoiceMeter from "./VoiceMeter.vue";
import { useGameStore } from "@/stores/game";

const store = useGameStore();
</script>

<template>
  <header class="app-header">
    <BrandMark />
    <nav class="primary-nav" aria-label="主导航">
      <RouterLink to="/"><Radio />牌局</RouterLink>
      <RouterLink to="/profile"><History />记录</RouterLink>
    </nav>
    <div class="header-account">
      <span class="connection-state" :class="{ offline: !store.backendOnline }"><Wifi />{{ store.backendOnline ? "服务已连接" : "本地演示" }}</span>
      <span class="score-readout"><small>局外积分</small><strong>{{ store.accountPoints.toLocaleString("zh-CN") }}</strong></span>
      <RouterLink class="icon-button" to="/rooms/new" title="创建牌局" aria-label="创建牌局"><Plus /></RouterLink>
      <button class="icon-button" type="button" title="通知" aria-label="通知"><Bell /></button>
      <RouterLink class="account-button" to="/profile"><CircleUserRound /><span>岱奇</span><VoiceMeter :active="store.microphoneEnabled" label="麦克风已开启" /></RouterLink>
    </div>
  </header>
</template>

