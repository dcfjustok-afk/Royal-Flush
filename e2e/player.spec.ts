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

test("积分账本短暂失败不会丢失账号登录状态", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖账号核心状态降级恢复");
  await page.route("**/api/v1/me", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      user: { id: "me", phone: "13800138000", nickname: "你", permissions: {}, banned: false, createdAt: "2026-08-24T12:00:00Z" },
      balance: 1860,
      activeRoomId: "",
    }),
  }));
  await page.route("**/api/v1/me/score-ledger", (route) => route.fulfill({
    status: 503,
    contentType: "application/json",
    body: JSON.stringify({ code: "temporarily_unavailable", message: "账本暂时不可用" }),
  }));

  await page.goto("/");
  await expect(page.getByRole("link", { name: "账号：你" })).toBeVisible();
  await expect(page.locator(".large-score")).toContainText("1,860");
  await expect(page.locator(".lobby-heading").getByRole("link", { name: "创建牌局" })).toBeVisible();
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

test("准备已提交时快照短暂失败不会误报操作失败", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖命令与快照恢复边界");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.players = [
    waiting.players.find((player) => player.id === "me")!,
    waiting.players.find((player) => player.id === "p1")!,
  ].map((player) => ({ ...player, isReady: false, isCurrentActor: false }));
  let commandApplied = false;
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => commandApplied
    ? route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ code: "temporarily_unavailable", message: "暂时不可用" }) })
    : route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(waiting) }));
  await page.route("**/api/v1/rooms/room-saturday/commands", async (route) => {
    commandApplied = true;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ duplicate: false, event: { type: "room.ready_changed" } }) });
  });

  await page.goto("/rooms/room-saturday/waiting");
  await page.getByRole("button", { name: /准备入局/ }).click();
  await expect(page.getByRole("button", { name: /已准备/ })).toBeVisible();
  await expect(page.locator(".form-message.error")).toHaveCount(0);
});

test("刷新陈旧房间地址会恢复到账号当前房间", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖跨标签切房后的刷新恢复");
  const target = structuredClone(playerSnapshot);
  target.roomId = "room-target";
  target.roomCode = "RF-TARG";
  target.roomName = "目标等候室";
  target.street = "waiting";
  target.handNumber = 0;
  target.board = [];
  target.holeCards = [];
  if (!target.config) throw new Error("player fixture is missing room config");
  target.config.name = "目标等候室";
  target.players = [
    target.players.find((player) => player.id === "me")!,
    target.players.find((player) => player.id === "p1")!,
  ].map((player, seat) => ({
    ...player,
    seat,
    isLocal: seat === 0,
    isCurrentActor: false,
    isReady: false,
  }));
  await page.route("**/api/v1/me", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      user: { id: "me", phone: "13800138000", nickname: "你", permissions: {}, banned: false, createdAt: "2026-08-24T12:00:00Z" },
      balance: 1860,
      activeRoomId: "room-target",
    }),
  }));
  await page.route("**/api/v1/rooms/room-old/snapshot", (route) => route.fulfill({
    status: 404,
    contentType: "application/json",
    body: JSON.stringify({ code: "player_not_seated", message: "你不在这个房间中" }),
  }));
  await page.route("**/api/v1/rooms/room-target/snapshot", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify(target),
  }));

  await page.goto("/rooms/room-old/waiting");
  await expect(page).toHaveURL(/\/rooms\/room-target\/waiting$/);
  await expect(page.getByRole("heading", { name: "目标等候室" })).toBeVisible();
  await expect(page.locator(".form-message.error")).toHaveCount(0);
});

test("刷新当前等候室会保留权威准备状态", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖同房刷新恢复");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.players = [
    waiting.players.find((player) => player.id === "me")!,
    waiting.players.find((player) => player.id === "p1")!,
  ].map((player) => ({
    ...player,
    isCurrentActor: false,
    isReady: true,
  }));
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify(waiting),
  }));

  await page.goto("/rooms/room-saturday/waiting");
  await expect(page.getByRole("button", { name: /已准备/ })).toBeVisible();
  await page.reload();
  await expect(page).toHaveURL(/\/rooms\/room-saturday\/waiting$/);
  await expect(page.getByRole("button", { name: /已准备/ })).toBeVisible();
  await expect(page.locator(".form-message.error")).toHaveCount(0);
});

test("准备请求进行中刷新会从服务端恢复最终状态", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖提交中刷新");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.players = [
    waiting.players.find((player) => player.id === "me")!,
    waiting.players.find((player) => player.id === "p1")!,
  ].map((player) => ({ ...player, isCurrentActor: false, isReady: false }));
  let commands = 0;
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify(waiting),
  }));
  await page.route("**/api/v1/rooms/room-saturday/commands", async (route) => {
    commands++;
    const local = waiting.players.find((player) => player.isLocal);
    if (local) local.isReady = true;
    await new Promise((resolve) => setTimeout(resolve, 250));
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ duplicate: false, event: { type: "room.ready_changed" } }) }).catch(() => undefined);
  });

  await page.goto("/rooms/room-saturday/waiting");
  await page.getByRole("button", { name: /准备入局/ }).click();
  await expect(page.getByRole("button", { name: /正在更新/ })).toBeDisabled();
  await page.reload();
  await expect(page.getByRole("button", { name: /已准备/ })).toBeVisible();
  await expect(page.locator(".form-message.error")).toHaveCount(0);
  expect(commands).toBe(1);
});

test("玩家可以从等候室主动离开且快速点击只提交一次", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖等候室离开流程");
  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.players.forEach((player) => {
    player.isCurrentActor = false;
    player.isReady = false;
  });
  let leaveCommands = 0;
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify(waiting),
  }));
  await page.route("**/api/v1/rooms/room-saturday/commands", async (route) => {
    const command = route.request().postDataJSON() as { type: string };
    if (command.type === "room.leave") leaveCommands++;
    await new Promise((resolve) => setTimeout(resolve, 80));
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ duplicate: false, event: { type: "room.player_leaving" } }) });
  });
  page.on("dialog", (dialog) => dialog.accept());

  await page.goto("/rooms/room-saturday/waiting");
  const leave = page.getByRole("button", { name: "离开当前房间" });
  await leave.evaluate((button: HTMLButtonElement) => {
    button.click();
    button.click();
  });
  await expect(page).toHaveURL(/\/$/);
  expect(leaveCommands).toBe(1);
  await expect(page.locator(".form-message.error")).toHaveCount(0);
});

test("跨房失败会保留原房间且重试成功后进入目标房间", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖跨房失败与重试");
  const oldRoom = structuredClone(playerSnapshot);
  oldRoom.roomId = "room-old";
  oldRoom.roomCode = "RF-OLD1";
  oldRoom.roomName = "原等候室";
  oldRoom.street = "waiting";
  oldRoom.handNumber = 0;
  oldRoom.players.forEach((player) => (player.isCurrentActor = false));
  const target = structuredClone(oldRoom);
  target.roomId = "room-target";
  target.roomCode = "RF-TARG";
  target.roomName = "目标等候室";
  if (!target.config) throw new Error("player fixture is missing room config");
  target.config.name = "目标等候室";
  target.players = [
    target.players.find((player) => player.id === "p1")!,
    target.players.find((player) => player.id === "me")!,
  ].map((player, seat) => ({ ...player, seat, isLocal: player.id === "me", isReady: false }));
  let joinAttempts = 0;
  await page.route("**/api/v1/me", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      user: { id: "me", phone: "13800138000", nickname: "你", permissions: {}, banned: false, createdAt: "2026-08-24T12:00:00Z" },
      balance: 1860,
      activeRoomId: "room-old",
    }),
  }));
  await page.route("**/api/v1/rooms/room-old/snapshot", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(oldRoom) }));
  await page.route("**/api/v1/rooms/RF-TARG/public", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      id: "room-target", code: "RF-TARG", name: "目标等候室", ownerId: "p1", ownerName: "阿桥",
      onlinePlayers: 1, maxPlayers: 8, occupiedSeats: [0], config: target.config,
    }),
  }));
  await page.route("**/api/v1/rooms/room-target/join", (route) => {
    joinAttempts++;
    if (joinAttempts === 1) {
      return route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({ code: "room_switch_in_hand", message: "当前手牌进行中，结束后才能切换房间" }),
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(target) });
  });
  await page.route("**/api/v1/rooms/room-target/snapshot", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(target) }));

  await page.goto("/invite/RF-TARG");
  await page.getByRole("button", { name: /进入等候室/ }).click();
  await expect(page).toHaveURL(/\/invite\/RF-TARG$/);
  await expect(page.getByRole("alert")).toContainText("当前手牌进行中");
  await page.getByRole("button", { name: /进入等候室/ }).click();
  await expect(page).toHaveURL(/\/rooms\/room-target\/waiting$/);
  await expect(page.getByRole("heading", { name: "目标等候室" })).toBeVisible();
  expect(joinAttempts).toBe(2);
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

test("牌桌行动快速双击只提交一次且不显示伪失败", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖牌桌行动并发");
  let actionCommands = 0;
  await page.route("**/api/v1/rooms/room-saturday/commands", async (route) => {
    const command = route.request().postDataJSON() as { type: string };
    if (command.type === "action.call") actionCommands++;
    await new Promise((resolve) => setTimeout(resolve, 80));
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ duplicate: false, event: { type: "game.action_applied" } }) });
  });

  await page.goto("/rooms/room-saturday/table");
  const call = page.getByRole("button", { name: "跟注 10" });
  await call.evaluate((button: HTMLButtonElement) => {
    button.click();
    button.click();
  });
  await expect(call).toBeEnabled();
  expect(actionCommands).toBe(1);
  await expect(page.locator(".table-state-banner")).toHaveCount(0);
});

test("房间状态与当前路由不一致时会进入正确页面", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1440", "代表性桌面项目覆盖跨设备开局与陈旧路由");

  await page.goto("/rooms/room-saturday/waiting");
  await expect(page).toHaveURL(/\/rooms\/room-saturday\/table$/);
  await expect(page.locator(".table-app")).toBeVisible();

  const waiting = structuredClone(playerSnapshot);
  waiting.street = "waiting";
  waiting.handNumber = 0;
  waiting.board = [];
  waiting.holeCards = [];
  waiting.players.forEach((player) => (player.isCurrentActor = false));
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify(waiting),
  }));
  await page.goto("/rooms/room-saturday/table");
  await expect(page).toHaveURL(/\/rooms\/room-saturday\/waiting$/);
  await expect(page.getByRole("heading", { name: "周六夜场" })).toBeVisible();
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
