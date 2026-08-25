import { readonly, ref } from "vue";

export type ThemeId = "obsidian" | "ivory" | "midnight";

export interface ThemeOption {
  id: ThemeId;
  label: string;
  description: string;
  colorScheme: "dark" | "light";
  themeColor: string;
}

export const themes: readonly ThemeOption[] = [
  { id: "obsidian", label: "黑曜牌室", description: "克制的黑色牌室与暖红点缀", colorScheme: "dark", themeColor: "#11110f" },
  { id: "ivory", label: "象牙俱乐部", description: "明亮纸感、深绿与朱砂红", colorScheme: "light", themeColor: "#f3efe4" },
  { id: "midnight", label: "午夜霓虹", description: "深海蓝、冰青与珊瑚光", colorScheme: "dark", themeColor: "#08131f" },
] as const;

const storageKey = "royal-flush-theme";
const defaultTheme: ThemeId = "obsidian";
const activeThemeState = ref<ThemeId>(defaultTheme);
let listeningForTabs = false;

function isThemeId(value: string | null): value is ThemeId {
  return themes.some((theme) => theme.id === value);
}

export function applyTheme(themeId: ThemeId, persist = true) {
  const theme = themes.find((candidate) => candidate.id === themeId) ?? themes[0]!;
  activeThemeState.value = theme.id;
  document.documentElement.dataset.theme = theme.id;
  document.documentElement.style.colorScheme = theme.colorScheme;
  document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.setAttribute("content", theme.themeColor);
  if (persist) window.localStorage.setItem(storageKey, theme.id);
}

export function initializeTheme() {
  const storedTheme = window.localStorage.getItem(storageKey);
  applyTheme(isThemeId(storedTheme) ? storedTheme : defaultTheme, false);
  if (!listeningForTabs) {
    window.addEventListener("storage", (event) => {
      if (event.key === storageKey && isThemeId(event.newValue)) applyTheme(event.newValue, false);
    });
    listeningForTabs = true;
  }
}

export function useTheme() {
  return { activeTheme: readonly(activeThemeState), themes, setTheme: applyTheme };
}
