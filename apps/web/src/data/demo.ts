import type { RoomConfig, ScoreLedgerEntry, TableSnapshot } from "@royal-flush/contracts";

export const demoRoomConfig: RoomConfig = {
  name: "周六夜场",
  maxPlayers: 8,
  blindPreset: "5/10",
  actionSeconds: 30,
  voiceEnabled: true,
  chipDenominations: [5, 10, 20, 50, 100],
};

export const demoSnapshot: TableSnapshot = {
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
    { id: "p1", name: "阿桥", initials: "桥", seat: 0, tablePoints: 1240, accountPoints: 3680, streetCommitted: 10, status: "active", isDealer: false, isSpeaking: true, isCurrentActor: false, isLocal: false, isReady: true },
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
  actionDeadline: new Date(Date.now() + 18_000).toISOString(),
};

export const demoLedger: ScoreLedgerEntry[] = [
  { id: "l4", type: "self_add", amount: 500, balance: 1860, roomId: "room-saturday", note: "自行增加积分", createdAt: new Date(Date.now() - 8 * 60_000).toISOString() },
  { id: "l3", type: "game_settlement", amount: -340, balance: 1360, roomId: "room-friday", note: "周五练习局净输赢", createdAt: new Date(Date.now() - 24 * 60 * 60_000).toISOString() },
  { id: "l2", type: "self_add", amount: 700, balance: 1700, note: "自行增加积分", createdAt: new Date(Date.now() - 3 * 24 * 60 * 60_000).toISOString() },
  { id: "l1", type: "initial_base", amount: 1000, balance: 1000, note: "账号初始积分", createdAt: new Date(Date.now() - 12 * 24 * 60 * 60_000).toISOString() },
];

export const demoMessages = [
  { id: "m1", type: "score", text: "小北自行增加了 500 积分，当前局外积分为 1,240", at: "22:48:16" },
  { id: "m2", type: "action", text: "阿桥加注 70，本轮总投入 80", at: "22:47:52" },
  { id: "m3", type: "system", text: "第 28 手牌开始，盲注 5 / 10", at: "22:46:34" },
];
