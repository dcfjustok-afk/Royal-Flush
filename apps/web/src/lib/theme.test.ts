import { beforeEach, describe, expect, it } from "vitest";
import { applyTheme, initializeTheme } from "./theme";

describe("player theme", () => {
  beforeEach(() => {
    const values = new Map<string, string>();
    Object.defineProperty(window, "localStorage", { configurable: true, value: {
      clear: () => values.clear(),
      getItem: (key: string) => values.get(key) ?? null,
      removeItem: (key: string) => values.delete(key),
      setItem: (key: string, value: string) => values.set(key, String(value)),
    } });
    document.documentElement.removeAttribute("data-theme");
    document.head.innerHTML = '<meta name="theme-color" content="#11110f">';
  });

  it("uses obsidian when no preference has been stored", () => {
    initializeTheme();
    expect(document.documentElement.dataset.theme).toBe("obsidian");
    expect(document.documentElement.style.colorScheme).toBe("dark");
  });

  it("persists an ivory preference and updates browser chrome", () => {
    applyTheme("ivory");
    expect(window.localStorage.getItem("royal-flush-theme")).toBe("ivory");
    expect(document.documentElement.dataset.theme).toBe("ivory");
    expect(document.documentElement.style.colorScheme).toBe("light");
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute("content")).toBe("#f3efe4");
  });

  it("restores a stored midnight preference", () => {
    window.localStorage.setItem("royal-flush-theme", "midnight");
    initializeTheme();
    expect(document.documentElement.dataset.theme).toBe("midnight");
    expect(document.documentElement.style.colorScheme).toBe("dark");
  });
});
