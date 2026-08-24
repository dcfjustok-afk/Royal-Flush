import { expect, test, type Locator, type Page } from "@playwright/test";
import { mockPlayerApi, playerSnapshot } from "./support/api-mocks";

async function expectInsideViewport(locator: Locator, page: Page) {
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width + 1);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height + 1);
}

test.beforeEach(async ({ page }) => {
  await mockPlayerApi(page);
  await page.emulateMedia({ reducedMotion: "reduce" });
});

test("玩家可以注册持久账号并进入大厅", async ({ page }, testInfo) => {
  await page.unroute("**/api/v1/**");
  await mockPlayerApi(page, { authenticated: false });
  await page.goto("/account");
  await expect(page.getByRole("heading", { name: "回到你的牌桌" })).toBeVisible();
  if (testInfo.project.name === "desktop-1440") await expect(page.locator(".account-page")).toHaveScreenshot("account.png");
  await page.getByRole("tab", { name: "注册" }).click();
  await page.getByLabel("显示名称").fill("新玩家");
  await page.getByLabel("手机号").fill("13800138000");
  await page.getByLabel("密码", { exact: true }).fill("table2026");
  await page.getByRole("button", { name: /注册并登录/ }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("link", { name: "账号：新玩家" })).toBeVisible();
});

test("跨设备登录会返回仍在等待的房间", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖账号房间恢复");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.board = [];
  waiting.holeCards = [];
  await page.route("**/api/v1/me", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      user: { id: "me", phone: "13800138000", nickname: "你", permissions: {}, banned: false, createdAt: "2026-08-24T12:00:00Z" },
      balance: 1860,
      activeRoomId: "room-saturday",
    }),
  }));
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(waiting) }));

  await page.goto("/");
  const resume = page.getByRole("link", { name: /返回等候室/ });
  await expect(resume).toBeVisible();
  await expect(resume).toHaveAttribute("href", "/rooms/room-saturday/waiting");
  await expect(page.locator(".room-signal")).toContainText("等待玩家准备");
});

test("断线的已准备玩家不会让房主误开局", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖多人准备状态");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  if (!waiting.config) throw new Error("player fixture is missing room config");
  waiting.config.maxPlayers = 2;
  waiting.players = [
    { ...waiting.players.find((player) => player.id === "me")!, isReady: true, isCurrentActor: false },
    { ...waiting.players.find((player) => player.id === "p1")!, isReady: true, status: "disconnected", isCurrentActor: false },
  ];
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(waiting) }));

  await page.goto("/rooms/room-saturday/waiting");
  await expect(page.getByText("1 人已准备")).toBeVisible();
  await expect(page.getByRole("button", { name: "等待至少两人准备" })).toBeDisabled();
});

test("快速重复点击准备只提交一次命令", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖准备按钮并发");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.players = [
    waiting.players.find((player) => player.id === "me")!,
    waiting.players.find((player) => player.id === "p1")!,
  ].map((player) => ({ ...player, isReady: false, isCurrentActor: false }));
  let commands = 0;
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(waiting) }));
  await page.route("**/api/v1/rooms/room-saturday/commands", async (route) => {
    commands++;
    const command = route.request().postDataJSON() as { type: string; payload: { ready?: boolean } };
    expect(command.type).toBe("room.ready");
    await new Promise((resolve) => setTimeout(resolve, 100));
    const local = waiting.players.find((player) => player.isLocal);
    if (local) local.isReady = command.payload.ready === true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ duplicate: false, event: { type: "room.ready_changed" } }) });
  });

  await page.goto("/rooms/room-saturday/waiting");
  await page.getByRole("button", { name: /准备入局/ }).evaluate((button: HTMLButtonElement) => {
    button.click();
    button.click();
  });
  await expect(page.getByRole("button", { name: /正在更新/ })).toBeDisabled();
  await expect(page.getByRole("button", { name: /已准备/ })).toBeVisible();
  expect(commands).toBe(1);
  await expect(page.locator(".form-message.error")).toHaveCount(0);
});

test("牌桌在目标视口内保持完整且筹码不会改变布局", async ({ page }) => {
  await page.goto("/rooms/room-saturday/table");
  await expect(page.locator(".table-app")).toBeVisible();
  await expect(page.locator(".community-cards .playing-card")).toHaveCount(5);
  await expect(page.locator(".network-status")).toContainText("连接稳定");
  await expect(page.locator(".station-voice")).toContainText("当前无人说话");
  await expect(page.locator(".player-seat.speaking")).toHaveCount(0);
  await expect(page.locator(".player-seat.acting")).toHaveAttribute("style", /--turn-progress: [0-9.]+turn/);

  const loadedFontFamilies = await page.evaluate(() => Array.from(document.fonts).filter((font) => font.status === "loaded").map((font) => font.family));
  expect(loadedFontFamilies).toContain("Noto Sans SC");
  expect(loadedFontFamilies).toContain("Barlow Condensed");

  const documentSize = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  expect(documentSize.width).toBeLessThanOrEqual(documentSize.viewport + 1);
  await expectInsideViewport(page.locator(".table-actions"), page);
  await expectInsideViewport(page.locator(".chip-controls"), page);
  await expectInsideViewport(page.locator(".table-tools"), page);

  const seat = await page.locator(".player-seat.local").boundingBox();
  const dock = await page.locator(".table-dock").boundingBox();
  const overlapsHorizontally = seat && dock && seat.x < dock.x + dock.width && seat.x + seat.width > dock.x;
  const overlapsVertically = seat && dock && seat.y < dock.y + dock.height && seat.y + seat.height > dock.y;
  expect(Boolean(overlapsHorizontally && overlapsVertically)).toBe(false);

  const chip = page.getByRole("button", { name: "增加 20 牌桌分" });
  const before = await chip.boundingBox();
  await chip.click();
  await expect(page.locator(".chip-selection")).toContainText("20");
  const after = await chip.boundingBox();
  expect({ width: after?.width, height: after?.height }).toEqual({ width: before?.width, height: before?.height });
  await expectInsideViewport(page.getByRole("button", { name: /确认加注 20/ }), page);
});

test("键盘可以完成房间码跳转和大额筹码配置", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖键盘工作流");
  await page.goto("/");
  const code = page.getByRole("textbox", { name: "房间码", exact: true });
  await code.focus();
  await code.fill("rf-a520");
  await code.press("Enter");
  await expect(page).toHaveURL(/\/invite\/RF-A520$/);

  await page.goto("/rooms/new");
  const largeChip = page.locator(".large-chip-options label").filter({ hasText: "500" });
  await largeChip.getByRole("checkbox").check();
  await expect(page.locator(".chip-preview")).toContainText("5 / 10 / 20 / 50 / 100 / 500");
});

test("桌内举报可通过设置面板完整提交", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖举报工作流");
  await page.goto("/rooms/room-saturday/table");
  await page.getByRole("button", { name: "牌桌设置" }).click();
  await page.getByRole("button", { name: /举报问题/ }).click();
  await page.getByLabel("问题类型").selectOption("voice");
  await page.getByLabel("相关玩家").selectOption("p2");
  await page.getByLabel("问题说明").fill("语音持续出现干扰");
  await page.getByRole("button", { name: "提交举报" }).click();
  await expect(page.locator(".report-feedback.success")).toContainText("举报已登记");
});

test("房主可以在结算后开始下一手", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖多人续局工作流");
  const settled = structuredClone(playerSnapshot);
  settled.street = "settled";
  settled.actionDeadline = "";
  settled.players.forEach((player) => (player.isCurrentActor = false));

  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(settled) }));
  await page.route("**/api/v1/rooms/room-saturday/commands", async (route) => {
    const command = route.request().postDataJSON() as { type: string };
    expect(command.type).toBe("game.start");
    settled.street = "preflop";
    settled.handNumber++;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ duplicate: false, event: { type: "game.hand_started" } }) });
  });

  await page.goto("/rooms/room-saturday/table");
  await expect(page.getByRole("button", { name: "开始下一手" })).toBeVisible();
  await page.getByRole("button", { name: "开始下一手" }).click();
  await expect(page.getByRole("button", { name: "开始下一手" })).toHaveCount(0);
  await expect(page.locator(".table-telemetry")).toContainText("029");
});

test("牌桌建立视觉回归基线", async ({ page }) => {
  await page.goto("/rooms/room-saturday/table");
  await page.addStyleTag({ content: ".action-timer strong,.action-timer i{visibility:hidden!important}.player-seat.acting{--turn-progress:.67turn!important}" });
  await expect(page.locator(".table-app")).toHaveScreenshot("table.png");
});
