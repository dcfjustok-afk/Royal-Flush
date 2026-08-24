import type { RoomConfig, TableSnapshot } from "@royal-flush/contracts";

export const emptyRoomConfig: RoomConfig = {
  name: "",
  maxPlayers: 8,
  blindPreset: "5/10",
  actionSeconds: 30,
  voiceEnabled: true,
  chipDenominations: [5, 10, 20, 50, 100],
};

export const emptySnapshot: TableSnapshot = {
  roomId: "",
  roomCode: "",
  roomName: "",
  version: 0,
  handNumber: 0,
  street: "waiting",
  pot: 0,
  board: [],
  holeCards: [],
  players: [],
  allowedChipDenominations: [5, 10, 20, 50, 100],
  toCall: 0,
  minimumRaiseBy: 0,
  maximumRaiseBy: 0,
  canCheck: false,
  canCall: false,
  canRaise: false,
  canAllIn: false,
  actionDeadline: "",
  config: emptyRoomConfig,
  messages: [],
};
