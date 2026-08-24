<script setup lang="ts">
import { Palette } from "@lucide/vue";
import type { ThemeId } from "@/lib/theme";
import { useTheme } from "@/lib/theme";

defineProps<{ compact?: boolean }>();

const { activeTheme, themes, setTheme } = useTheme();

function selectTheme(event: Event) {
  setTheme((event.target as HTMLSelectElement).value as ThemeId);
}
</script>

<template>
  <label class="theme-switcher" :class="{ compact }" :title="themes.find((theme) => theme.id === activeTheme)?.description">
    <Palette aria-hidden="true" />
    <span class="sr-only">界面主题</span>
    <select name="interface-theme" aria-label="界面主题" :value="activeTheme" @change="selectTheme">
      <option v-for="theme in themes" :key="theme.id" :value="theme.id">{{ theme.label }}</option>
    </select>
  </label>
</template>
