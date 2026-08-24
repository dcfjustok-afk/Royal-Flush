import { expect, test, type Page } from "@playwright/test";
import { mockAdminApi, mockPlayerApi } from "./support/api-mocks";

async function expectNoPageOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({ page: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  expect(dimensions.page).toBeLessThanOrEqual(dimensions.viewport + 1);
}

async function expectUsableControls(page: Page) {
  const undersized = await page.locator("button, input, select").evaluateAll((controls) => controls
    .filter((control) => {
      const element = control as HTMLElement;
      const style = getComputedStyle(element);
      return style.display !== "none" && style.visibility !== "hidden" && element.getBoundingClientRect().width > 0;
    })
    .map((control) => {
      const element = control as HTMLElement;
      const rect = element.getBoundingClientRect();
      return { label: element.getAttribute("aria-label") || element.textContent?.trim() || element.tagName, width: rect.width, height: rect.height };
    })
    .filter((control) => control.width < 36 || control.height < 36));
  expect(undersized).toEqual([]);
}

test("玩家端主题可切换、持久化且控件尺寸可用", async ({ page }) => {
  await mockPlayerApi(page);
  await page.goto("/");
  const picker = page.getByLabel("界面主题").first();
  await expect(picker).toBeVisible();

  await picker.selectOption("ivory");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "ivory");
  await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute("content", "#f3efe4");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "ivory");

  await picker.selectOption("midnight");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "midnight");
  await picker.selectOption("obsidian");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "obsidian");
  await expectNoPageOverflow(page);
  await expectUsableControls(page);
});

test("运营端主题和窄屏布局保持可用", async ({ page }) => {
  await mockAdminApi(page);
  await page.goto("http://127.0.0.1:43174");
  await expect(page.getByRole("heading", { name: "运行概览" })).toBeVisible();
  const picker = page.getByLabel("界面主题").first();

  await picker.selectOption("midnight");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "midnight");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "midnight");
  await picker.selectOption("ivory");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "ivory");
  await expectNoPageOverflow(page);
  await expectUsableControls(page);
});
