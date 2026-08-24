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

test("运营搜索和房间详情只接受最后一次异步响应", async ({ page }) => {
  const user = (id: string, nickname: string) => ({
    id, nickname, phone: "13800138000", balance: 1000, banned: false,
    createdAt: "2026-08-24T12:00:00Z", updatedAt: "2026-08-24T12:00:00Z",
  });
  await page.route("**/api/v1/admin/users?*", async (route) => {
    const query = new URL(route.request().url()).searchParams.get("q") ?? "";
    if (query === "慢") await new Promise((resolve) => setTimeout(resolve, 500));
    if (query === "快") await new Promise((resolve) => setTimeout(resolve, 20));
    const users = query === "慢" ? [user("slow-user", "慢结果")] : query === "快" ? [user("fast-user", "快结果")] : [];
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ users }) });
  });
  await page.route("**/api/v1/admin/rooms/room-demo-*", async (route) => {
    const roomId = new URL(route.request().url()).pathname.split("/").at(-1)!;
    const slow = roomId === "room-demo-2806";
    await new Promise((resolve) => setTimeout(resolve, slow ? 300 : 20));
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        roomId, roomCode: slow ? "RF-2806" : "RF-9132", roomName: slow ? "慢房间详情" : "快房间详情",
        ownerId: "u1", version: 1, handNumber: 1, street: "waiting", pot: 0, players: [],
        config: { blindPreset: "5/10", actionSeconds: 30, voiceEnabled: true, chipDenominations: [5, 10, 20, 50, 100] }, messages: [],
      }),
    });
  });

  await page.getByRole("button", { name: /用户与积分/ }).click();
  const search = page.getByLabel("搜索用户");
  await search.fill("慢");
  await page.waitForTimeout(350);
  await search.fill("快");
  await expect(page.getByText("快结果")).toBeVisible();
  await page.waitForTimeout(500);
  await expect(page.getByText("快结果")).toBeVisible();
  await expect(page.getByText("慢结果")).toHaveCount(0);

  await page.getByRole("button", { name: /活跃房间/ }).click();
  await page.getByRole("button", { name: /周六夜场/ }).click();
  await page.getByRole("button", { name: /慢速夜场/ }).click();
  await expect(page.getByRole("heading", { name: "快房间详情" })).toBeVisible();
  await page.waitForTimeout(350);
  await expect(page.getByRole("heading", { name: "快房间详情" })).toBeVisible();

  await page.getByRole("button", { name: /周六夜场/ }).click();
  await page.getByRole("button", { name: "关闭房间详情" }).click();
  await page.waitForTimeout(350);
  await expect(page.locator(".detail-drawer")).toHaveCount(0);
});
