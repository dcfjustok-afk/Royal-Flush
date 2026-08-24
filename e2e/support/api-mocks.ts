import type { Page, Route } from "@playwright/test";
import type { TableSnapshot } from "@royal-flush/contracts";

const json = (route: Route, body: unknown, status = 200) => route.fulfill({
  status,
  contentType: "application/json",
  body: JSON.stringify(body),
});

export const playerSnapshot: TableSnapshot = {
  roomId: "room-saturday",
  roomCode: "RF-2806",
  roomName: "周六夜场",
  ownerId: "me",
  version: 28,
  handNumber: 28,
  street: "river",
  pot: 145,
  board: [
    { rank: "A", suit: "hearts" },
    { rank: "9", suit: "clubs" },
    { rank: "7", suit: "diamonds" },
    { rank: "K", suit: "spades" },
    { rank: "3", suit: "hearts" },
  ],
  holeCards: [
    { rank: "A", suit: "spades" },
    { rank: "Q", suit: "hearts" },
  ],
  players: [
    { id: "p1", name: "阿桥", initials: "桥", seat: 0, tablePoints: 1240, accountPoints: 3680, streetCommitted: 10, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: false, isLocal: false, isReady: true },
    { id: "p2", name: "许栗", initials: "栗", seat: 1, tablePoints: 835, accountPoints: 920, streetCommitted: 10, status: "active", isDealer: true, isSpeaking: false, isCurrentActor: false, isLocal: false, isReady: true },
    { id: "p3", name: "小北", initials: "北", seat: 2, tablePoints: 1090, accountPoints: 1240, streetCommitted: 10, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: false, isLocal: false },
    { id: "p4", name: "远山", initials: "山", seat: 3, tablePoints: 680, accountPoints: -240, streetCommitted: 10, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: false, isLocal: false },
    { id: "p5", name: "林度", initials: "林", seat: 6, tablePoints: 755, accountPoints: 610, streetCommitted: 10, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: false, isLocal: false },
    { id: "p6", name: "南序", initials: "南", seat: 7, tablePoints: 1315, accountPoints: 2100, streetCommitted: 10, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: false, isLocal: false },
    { id: "me", name: "你", initials: "你", seat: 5, tablePoints: 960, accountPoints: 1860, streetCommitted: 0, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: true, isLocal: true },
  ],
  allowedChipDenominations: [5, 10, 20, 50, 100],
  toCall: 10,
  minimumRaiseBy: 10,
  maximumRaiseBy: 950,
  canCheck: false,
  canCall: true,
  canRaise: true,
  canAllIn: true,
  actionDeadline: "2099-01-01T00:00:00.000Z",
  config: {
    name: "周六夜场",
    maxPlayers: 8,
    blindPreset: "5/10",
    actionSeconds: 30,
    voiceEnabled: true,
    chipDenominations: [5, 10, 20, 50, 100],
  },
  messages: [
    { id: "m1", type: "score", text: "小北自行增加了 500 积分，当前局外积分为 1,240", createdAt: "2026-08-24T14:48:16+08:00" },
    { id: "m2", type: "action", text: "阿桥加注 70，本轮总投入 80", createdAt: "2026-08-24T14:47:52+08:00" },
    { id: "m3", type: "system", text: "第 28 手牌开始，盲注 5 / 10", createdAt: "2026-08-24T14:46:34+08:00" },
  ],
};

export async function mockPlayerApi(page: Page) {
  await page.addInitScript(() => {
    class StableWebSocket extends EventTarget {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSING = 2;
      static readonly CLOSED = 3;
      readonly url: string;
      readyState = StableWebSocket.CONNECTING;

      constructor(url: string | URL) {
        super();
        this.url = String(url);
        window.setTimeout(() => {
          this.readyState = StableWebSocket.OPEN;
          this.dispatchEvent(new Event("open"));
        }, 0);
      }

      close() {
        this.readyState = StableWebSocket.CLOSED;
        this.dispatchEvent(new CloseEvent("close"));
      }

      send() {}
    }
    Object.defineProperty(window, "WebSocket", { configurable: true, value: StableWebSocket });
  });

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/v1/health") return json(route, { status: "ok" });
    if (path === "/api/v1/me") return json(route, { user: { id: "me", nickname: "你" }, balance: 1860, activeRoomId: "" });
    if (path === "/api/v1/me/score-ledger") return json(route, { balance: 1860, entries: [] });
    if (path.endsWith("/snapshot")) return json(route, playerSnapshot);
    if (path.endsWith("/public")) return json(route, {
      id: "room-saturday",
      code: "RF-A520",
      name: "周六夜场",
      ownerId: "me",
      ownerName: "你",
      onlinePlayers: 7,
      maxPlayers: 8,
      occupiedSeats: [0, 1, 2, 3, 5, 6, 7],
      config: playerSnapshot.config,
    });
    if (path.endsWith("/commands") && request.method() === "POST") return json(route, {
      duplicate: false,
      event: { id: "event-e2e", roomId: "room-saturday", sequence: 29, type: "action.accepted", payload: {}, createdAt: "2026-08-24T14:49:00+08:00" },
    });
    if (path === "/api/v1/reports" && request.method() === "POST") return json(route, {
      duplicate: false,
      report: { id: "report-e2e", reporterId: "me", roomId: "room-saturday", subjectUserId: "p2", category: "voice", detail: "语音持续出现干扰", status: "open", createdAt: "2026-08-24T14:49:00+08:00" },
    });
    if (path.endsWith("/voice-token")) return json(route, { enabled: false, reason: "语音服务未配置" });

    return json(route, { code: "unmocked_e2e_request", message: `未模拟 ${request.method()} ${path}` }, 501);
  });
}

const timestamp = "2026-08-24T12:00:00.000Z";
const adminRooms = [
  { id: "room-demo-2806", code: "RF-2806", name: "周六夜场", ownerId: "u1", ownerName: "岱奇", players: 6, onlinePlayers: 6, maxPlayers: 8, blindPreset: "5/10", handNumber: 28, voiceEnabled: true, status: "playing", version: 94, createdAt: timestamp },
  { id: "room-demo-9132", code: "RF-9132", name: "慢速夜场", ownerId: "u2", ownerName: "阿桥", players: 4, onlinePlayers: 3, maxPlayers: 6, blindPreset: "2/5", handNumber: 11, voiceEnabled: true, status: "playing", version: 47, createdAt: timestamp },
  { id: "room-demo-0475", code: "RF-0475", name: "练习桌", ownerId: "u3", ownerName: "小北", players: 2, onlinePlayers: 2, maxPlayers: 8, blindPreset: "10/20", handNumber: 0, voiceEnabled: false, status: "waiting", version: 8, createdAt: timestamp },
];
const adminUsers = [
  { id: "u1", nickname: "岱奇", phone: "13800132806", balance: 1860, activeRoomId: "room-demo-2806", banned: false, createdAt: timestamp, updatedAt: timestamp },
  { id: "u2", nickname: "阿桥", phone: "13900131408", balance: 3680, activeRoomId: "room-demo-2806", banned: false, createdAt: timestamp, updatedAt: timestamp },
  { id: "u3", nickname: "远山", phone: "18600139021", balance: -240, activeRoomId: "room-demo-9132", banned: false, createdAt: timestamp, updatedAt: timestamp },
  { id: "u4", nickname: "林度", phone: "13700136110", balance: 610, banned: true, createdAt: timestamp, updatedAt: timestamp },
];
const adminReports = [
  { id: "report-demo-1", reporterId: "u3", roomId: "room-demo-9132", subjectUserId: "u2", category: "conduct", detail: "连续多轮在最后一秒行动，影响牌局节奏。", status: "open", createdAt: timestamp },
  { id: "report-demo-2", reporterId: "u1", roomId: "room-demo-2806", category: "voice", detail: "语音中持续出现杂音。", status: "open", createdAt: timestamp },
];
const adminAudits = [
  { id: "audit-demo-1", administratorId: "ops.li", action: "score.reset_all", targetType: "score_epoch", targetId: "4", reason: "季度演练", requestId: "demo-1", metadata: {}, createdAt: "2026-08-20T02:34:00Z" },
  { id: "audit-demo-2", administratorId: "ops.chen", action: "user.unban", targetType: "user", targetId: "u4", reason: "复核完成", requestId: "demo-2", metadata: {}, createdAt: "2026-08-18T09:20:00Z" },
];

export async function mockAdminApi(page: Page, options: { authenticated?: boolean } = {}) {
  let authenticated = options.authenticated ?? true;
  const users = structuredClone(adminUsers);
  const reports = structuredClone(adminReports);

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const operator = { id: "ops-e2e", phone: "", nickname: "ops-e2e", permissions: { "admin:read": true, "report:manage": true } };

    if (path === "/api/v1/auth/password/login" && request.method() === "POST") {
      authenticated = true;
      return json(route, { user: operator, balance: 1000 });
    }
    if (path === "/api/v1/me") {
      return authenticated ? json(route, { user: operator, balance: 1000, activeRoomId: "" }) : json(route, { code: "unauthorized", message: "请先登录" }, 401);
    }
    if (!authenticated) return json(route, { code: "unauthorized", message: "请先登录" }, 401);
    if (path === "/api/v1/admin/score-epochs") return json(route, { epochs: [{ id: 4, reason: "季度演练", administrator: "ops.li", createdAt: timestamp }] });
    if (path === "/api/v1/admin/rooms") return json(route, { rooms: adminRooms });
    if (path === "/api/v1/admin/users") return json(route, { users });
    if (path === "/api/v1/admin/reports") return json(route, { reports });
    if (path === "/api/v1/admin/audit-log") return json(route, { audits: adminAudits });
    if (path === "/api/v1/admin/score-resets" && request.method() === "POST") return json(route, { epoch: 5, baseScore: 1000, duplicate: false });
    if (path.endsWith("/ban-actions") && request.method() === "POST") {
      const id = decodeURIComponent(path.split("/").at(-2) ?? "");
      const user = users.find((candidate) => candidate.id === id)!;
      user.banned = !user.banned;
      return json(route, { user, duplicate: false });
    }
    if (path.endsWith("/resolution") && request.method() === "POST") {
      const id = decodeURIComponent(path.split("/").at(-2) ?? "");
      const report = reports.find((candidate) => candidate.id === id)!;
      report.status = "resolved";
      return json(route, { report, duplicate: false });
    }

    return json(route, { code: "unmocked_e2e_request", message: `未模拟 ${request.method()} ${path}` }, 501);
  });
}
