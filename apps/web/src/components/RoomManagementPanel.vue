<script setup lang="ts">
import { computed, ref } from "vue";
import { DoorClosed, Mic, MicOff, RefreshCw, ShieldCheck, UserMinus } from "@lucide/vue";
import { useGameStore } from "@/stores/game";

const emit = defineEmits<{ roomEnded: [] }>();
const store = useGameStore();
const busyCommand = ref("");
const feedback = ref("");
const isOwner = computed(() => Boolean(store.localPlayer && store.snapshot.ownerId === store.localPlayer.id));
const managedPlayers = computed(() => store.snapshot.players.filter((player) => !player.isLocal));

async function execute(type: string, payload: Record<string, unknown>, success: string) {
  if (busyCommand.value) return;
  busyCommand.value = type;
  feedback.value = "";
  try {
    await store.sendCommand(type, payload);
    feedback.value = success;
    return true;
  } catch (reason) {
    feedback.value = reason instanceof Error ? reason.message : "房间操作失败";
    return false;
  } finally {
    busyCommand.value = "";
  }
}

async function removePlayer(userId: string, name: string) {
  if (!window.confirm(`将 ${name} 移出房间？`)) return;
  await execute("room.remove_player", { userId }, `${name} 已被移出房间`);
}

async function transferOwner(userId: string, name: string) {
  if (!window.confirm(`将房主转让给 ${name}？`)) return;
  await execute("room.transfer_owner", { userId }, `已将房主转让给 ${name}`);
}

async function endRoom() {
  if (!window.confirm("结束房间并结算所有座位？进行中的一手牌不能直接结束。")) return;
  if (await execute("room.end", {}, "房间已结束并完成结算")) emit("roomEnded");
}
</script>

<template>
  <section v-if="isOwner" class="room-management">
    <header><h3>房主管理</h3><ShieldCheck /></header>
    <ul v-if="managedPlayers.length" class="managed-player-list">
      <li v-for="player in managedPlayers" :key="player.id">
        <span><strong>{{ player.name }}</strong><small>{{ player.status === "disconnected" ? "已断线" : player.isMuted ? "已禁言" : "在桌" }}</small></span>
        <div>
          <button class="icon-button compact" type="button" :disabled="Boolean(busyCommand)" :title="player.isMuted ? `解除 ${player.name} 的禁言` : `将 ${player.name} 禁言`" :aria-label="player.isMuted ? `解除 ${player.name} 的禁言` : `将 ${player.name} 禁言`" @click="execute('voice.mute', { userId: player.id, muted: !player.isMuted }, player.isMuted ? `${player.name} 已解除禁言` : `${player.name} 已禁言`)"><Mic v-if="player.isMuted" /><MicOff v-else /></button>
          <button class="icon-button compact" type="button" :disabled="Boolean(busyCommand)" :title="`转让房主给 ${player.name}`" :aria-label="`转让房主给 ${player.name}`" @click="transferOwner(player.id, player.name)"><ShieldCheck /></button>
          <button class="icon-button compact danger" type="button" :disabled="Boolean(busyCommand)" :title="`移出 ${player.name}`" :aria-label="`移出 ${player.name}`" @click="removePlayer(player.id, player.name)"><UserMinus /></button>
        </div>
      </li>
    </ul>
    <p v-else class="management-empty">暂无其他玩家</p>
    <div class="management-actions">
      <button class="tool-button" type="button" :disabled="Boolean(busyCommand)" @click="execute('room.rotate_invite', {}, '邀请链接已更新')"><RefreshCw />更新邀请链接</button>
      <button class="tool-button danger" type="button" :disabled="Boolean(busyCommand)" @click="endRoom"><DoorClosed />结束房间</button>
    </div>
    <p v-if="feedback" class="management-feedback" role="status">{{ feedback }}</p>
  </section>
</template>
