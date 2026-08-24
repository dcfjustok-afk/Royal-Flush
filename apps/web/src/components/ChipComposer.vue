<script setup lang="ts">
import type { ChipDenomination } from "@royal-flush/contracts";
import { Bomb, RotateCcw, Undo2, X } from "@lucide/vue";
import { computed, ref } from "vue";

const props = defineProps<{
  denominations: ChipDenomination[];
  toCall: number;
  minimumRaiseBy: number;
  maximumRaiseBy: number;
  canCheck: boolean;
  canRaise: boolean;
  canAllIn: boolean;
  disabled?: boolean;
}>();
const emit = defineEmits<{ raise: [chips: ChipDenomination[]]; call: []; fold: []; allIn: [] }>();
const selected = ref<ChipDenomination[]>([]);
const raiseBy = computed(() => selected.value.reduce<number>((sum, chip) => sum + chip, 0));
const raiseTo = computed(() => props.toCall + raiseBy.value);
const shortfall = computed(() => Math.max(0, props.minimumRaiseBy - raiseBy.value));
const valid = computed(() => props.canRaise && raiseBy.value >= props.minimumRaiseBy && raiseBy.value <= props.maximumRaiseBy && !props.disabled);

function add(chip: ChipDenomination) {
  if (raiseBy.value + chip <= props.maximumRaiseBy) selected.value.push(chip);
}
function undo() { selected.value.pop(); }
function clear() { selected.value = []; }
function confirm() {
  if (!valid.value) return;
  emit("raise", [...selected.value]);
  clear();
}
</script>

<template>
  <section class="chip-composer" :aria-disabled="disabled">
    <header class="dock-heading"><span>选择加注筹码</span><strong>可重复组合</strong></header>
    <div class="chip-controls" aria-label="可用筹码">
      <button v-for="denomination in denominations" :key="denomination" class="poker-chip" :class="`chip-${denomination}`" type="button" :disabled="disabled || raiseBy + denomination > maximumRaiseBy" :aria-label="`增加 ${denomination} 牌桌分`" @click="add(denomination)"><span>{{ denomination }}</span></button>
    </div>
    <div class="chip-selection">
      <span>{{ selected.length ? selected.join(" + ") : "尚未选择筹码" }}</span>
      <strong>加注 {{ raiseBy.toLocaleString("zh-CN") }}</strong>
      <span v-if="shortfall">还差 {{ shortfall }}</span><span v-else>满足最小加注</span>
      <button class="mini-icon" type="button" :disabled="!selected.length" title="撤回最后一枚" aria-label="撤回最后一枚筹码" @click="undo"><Undo2 /></button>
      <button class="mini-icon" type="button" :disabled="!selected.length" title="清空筹码" aria-label="清空已选筹码" @click="clear"><X /></button>
    </div>
    <div class="action-summary">
      <span><small>需要跟注</small><strong>{{ toCall }}</strong></span>
      <span><small>额外加注</small><strong>{{ raiseBy }}</strong></span>
      <span><small>本轮总投入</small><strong>{{ raiseTo }}</strong></span>
    </div>
    <div class="table-actions">
      <button class="table-action quiet" type="button" :disabled="disabled" @click="emit('fold')">弃牌</button>
      <button class="table-action call" type="button" :disabled="disabled" @click="emit('call')">{{ canCheck ? "过牌" : `跟注 ${toCall}` }}</button>
      <button class="table-action raise" type="button" :disabled="!valid" @click="confirm">确认加注 {{ raiseBy }}</button>
      <button class="table-action all-in" type="button" :disabled="disabled || !canAllIn" @click="emit('allIn')"><Bomb />全下</button>
    </div>
    <button v-if="disabled" class="waiting-action" type="button" disabled><RotateCcw />等待其他玩家行动</button>
  </section>
</template>

