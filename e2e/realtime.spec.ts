import { expect, test } from "@playwright/test";

test("WebSocket 中断后自动重连并恢复权威快照", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-1280", "单一项目验证时序，避免重复连接压力");

  await page.addInitScript(() => {
    class ControlledWebSocket extends EventTarget {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSING = 2;
      static readonly CLOSED = 3;
      readonly url: string;
      readyState = ControlledWebSocket.CONNECTING;

      constructor(url: string | URL) {
        super();
        this.url = String(url);
        const count = ((window as unknown as { __wsCreated?: number }).__wsCreated ?? 0) + 1;
        (window as unknown as { __wsCreated: number }).__wsCreated = count;
        (window as unknown as { __lastSocket: ControlledWebSocket }).__lastSocket = this;
        window.setTimeout(() => {
          this.readyState = ControlledWebSocket.OPEN;
          this.dispatchEvent(new Event("open"));
        }, 0);
      }

      forceClose() {
        if (this.readyState === ControlledWebSocket.CLOSED) return;
        this.readyState = ControlledWebSocket.CLOSED;
        this.dispatchEvent(new CloseEvent("close"));
      }

      close() { this.forceClose(); }

      send() {}
    }
    Object.defineProperty(window, "WebSocket", { configurable: true, value: ControlledWebSocket });
  });

  let snapshots = 0;
  await page.route("**/api/v1/health", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ status: "ok" }) }));
  await page.route("**/api/v1/rooms/room-saturday/snapshot", (route) => {
    snapshots += 1;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
      roomId: "room-saturday", roomCode: "RF-2806", roomName: "周六夜场", ownerId: "me", version: snapshots,
      handNumber: 28, street: "river", pot: 145, board: [], holeCards: [], allowedChipDenominations: [5, 10, 20, 50, 100],
      toCall: 10, minimumRaiseBy: 10, maximumRaiseBy: 950, canCheck: false, canCall: true, canRaise: true, canAllIn: true,
      actionDeadline: new Date(Date.now() + 30_000).toISOString(), config: { name: "周六夜场", maxPlayers: 8, blindPreset: "5/10", actionSeconds: 30, voiceEnabled: true, chipDenominations: [5, 10, 20, 50, 100] },
      players: [{ id: "me", name: "你", initials: "你", seat: 5, tablePoints: 1000, accountPoints: 1000, streetCommitted: 0, status: "active", isDealer: true, isSpeaking: false, isCurrentActor: true, isLocal: true }],
    }) });
  });

  await page.goto("http://127.0.0.1:5175/rooms/room-saturday/table");
  await expect(page.locator(".network-status")).toContainText("连接稳定");
  await expect.poll(() => snapshots).toBeGreaterThanOrEqual(1);
  await page.evaluate(() => (window as unknown as { __lastSocket: { forceClose: () => void } }).__lastSocket.forceClose());
  await expect.poll(() => page.evaluate(() => (window as unknown as { __wsCreated?: number }).__wsCreated ?? 0).catch(() => 0)).toBeGreaterThanOrEqual(2);
  await expect.poll(() => snapshots).toBeGreaterThanOrEqual(2);
  await expect(page.locator(".network-status")).toContainText("连接稳定");
});
