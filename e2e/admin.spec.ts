import { expect, test } from "@playwright/test";
import { mockAdminApi } from "./support/api-mocks";

test.beforeEach(async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "运营台以桌面端为目标");
  await mockAdminApi(page, { authenticated: !testInfo.title.includes("账号密码") });
  await page.goto("http://127.0.0.1:43174");
});

test("运营管理员可以用账号密码登录", async ({ page }) => {
  await page.getByLabel("管理员账号").fill("ops-e2e");
  await page.getByLabel("管理员密码").fill("correct-horse-battery-staple");
  await page.getByRole("button", { name: "登录运营台" }).click();
  await expect(page.getByRole("heading", { name: "运行概览" })).toBeVisible();
});

test("运营管理员可以审计式解封并处理举报", async ({ page }) => {
  await page.getByRole("button", { name: /用户与积分/ }).click();
  await page.getByRole("button", { name: "解除封禁" }).click();
  const moderation = page.getByRole("dialog", { name: "解除用户封禁" });
  await moderation.getByLabel("操作原因").fill("复核记录无异常");
  await moderation.getByRole("button", { name: "确认解封" }).click();
  await expect(moderation).toBeHidden();

  await page.getByRole("button", { name: /举报处理/ }).click();
  await page.locator(".report-list article").first().getByRole("button", { name: "处理" }).click();
  const report = page.getByRole("dialog", { name: "处理举报" });
  await report.getByLabel("处理原因").fill("已完成桌内记录核查");
  await report.getByRole("button", { name: "提交处理结果" }).click();
  await expect(report).toBeHidden();
  await expect(page.locator(".report-list article").first()).toContainText("已解决");
});

test("全站积分重置要求原因和确认短语", async ({ page }) => {
  await page.getByRole("button", { name: "重置全站积分" }).first().click();
  const dialog = page.getByRole("dialog", { name: "重置全站积分" });
  const confirm = dialog.getByRole("button", { name: "确认重置" });
  await expect(confirm).toBeDisabled();
  await dialog.getByLabel("重置原因").fill("季度积分演练");
  await dialog.getByLabel(/RESET ALL SCORES/).fill("RESET ALL SCORES");
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(dialog).toContainText("重置已完成");
});

test("运营概览建立视觉回归基线", async ({ page }) => {
  const loadedFontFamilies = await page.evaluate(() => Array.from(document.fonts).filter((font) => font.status === "loaded").map((font) => font.family));
  expect(loadedFontFamilies).toContain("Noto Sans SC");
  expect(loadedFontFamilies).toContain("Barlow Condensed");
  await page.addStyleTag({ content: "time,.admin-topbar>div:first-child>span{visibility:hidden!important}" });
  await expect(page.locator(".admin-shell")).toHaveScreenshot("admin-overview.png");
});
