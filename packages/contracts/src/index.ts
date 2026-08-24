export const BASE_CHIP_DENOMINATIONS = [5, 10, 20, 50, 100] as const;
export const LARGE_CHIP_DENOMINATIONS = [200, 500, 1000] as const;
export type ChipDenomination = 5 | 10 | 20 | 50 | 100 | 200 | 500 | 1000;
export type BlindPreset = "2/5" | "5/10" | "10/20";

export interface RoomConfig {
  name: string;
  maxPlayers: number;
  blindPreset: BlindPreset;
  actionSeconds: 20 | 30 | 45;
  voiceEnabled: boolean;
  chipDenominations: ChipDenomination[];
}

export interface PlayerSnapshot {
  id: string;
  name: string;
  initials: string;
  seat: number;
  tablePoints: number;
  accountPoints: number;
  streetCommitted: number;
  status: "active" | "folded" | "all-in" | "away" | "disconnected";
  isDealer: boolean;
  isSpeaking: boolean;
  isCurrentActor: boolean;
  isLocal: boolean;
}

export interface PlayingCard {
  rank: "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" | "10" | "J" | "Q" | "K" | "A";
  suit: "spades" | "hearts" | "diamonds" | "clubs";
}

export interface TableSnapshot {
  roomId: string;
  roomCode: string;
  roomName: string;
  version: number;
  handNumber: number;
  street: "waiting" | "preflop" | "flop" | "turn" | "river" | "showdown" | "settled";
  pot: number;
  board: PlayingCard[];
  holeCards: PlayingCard[];
  players: PlayerSnapshot[];
  allowedChipDenominations: ChipDenomination[];
  toCall: number;
  minimumRaiseBy: number;
  maximumRaiseBy: number;
  canCheck: boolean;
  canCall: boolean;
  canRaise: boolean;
  canAllIn: boolean;
  actionDeadline: string;
}

export interface ScoreLedgerEntry {
  id: string;
  type: "initial_base" | "self_add" | "game_settlement" | "admin_reset";
  amount: number;
  balance: number;
  roomId?: string;
  note: string;
  createdAt: string;
}

export interface RoomEvent<TPayload = unknown> {
  type: string;
  requestId: string;
  roomId: string;
  version: number;
  sentAt: string;
  payload: TPayload;
}

export interface RaiseCommand {
  chips: ChipDenomination[];
  expectedVersion: number;
  requestId: string;
}

export interface ScoreAdditionRequest {
  amount: number;
  roomId?: string;
  requestId: string;
}

export interface ScoreResetRequest {
  reason: string;
  confirmation: "RESET ALL SCORES";
  requestId: string;
}

