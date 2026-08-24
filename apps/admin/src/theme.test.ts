import { beforeEach, describe, expect, it } from "vitest";
import { applyTheme, initializeTheme } from "./theme";

describe("operator theme", () => {
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
  });

  it("persists a midnight preference and updates browser chrome", () => {
    applyTheme("midnight");
    expect(window.localStorage.getItem("royal-flush-theme")).toBe("midnight");
    expect(document.documentElement.dataset.theme).toBe("midnight");
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute("content")).toBe("#08131f");
  });

  it("restores a stored ivory preference", () => {
    window.localStorage.setItem("royal-flush-theme", "ivory");
    initializeTheme();
    expect(document.documentElement.dataset.theme).toBe("ivory");
    expect(document.documentElement.style.colorScheme).toBe("light");
  });
});
