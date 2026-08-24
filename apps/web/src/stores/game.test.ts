import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it } from "vitest";
import { emptySnapshot } from "@/data/empty";
import { reconnectDelay, useGameStore } from "./game";

describe("game store realtime recovery", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("backs off quickly while keeping every retry within the recovery ceiling", () => {
    expect([0, 1, 2, 3, 8].map(reconnectDelay)).toEqual([250, 500, 1000, 2000, 2000]);
  });

  it("starts without players, messages, ledger records, or a local score", () => {
    const store = useGameStore();

    expect(store.accountPoints).toBeNull();
    expect(store.snapshot.players).toEqual([]);
    expect(store.messages).toEqual([]);
    expect(store.ledger).toEqual([]);
  });

  it("accepts persisted messages from the authoritative snapshot", () => {
    const store = useGameStore();
    store.acceptSnapshot({
      ...structuredClone(emptySnapshot),
      roomId: "room-live",
      roomCode: "RF-LIVE",
      roomName: "真实牌局",
      version: 7,
      messages: [{
        id: "message-live",
        type: "score",
        text: "许棠自行增加了 1000 积分，当前局外积分为 1920",
        createdAt: "2026-08-24T14:08:09Z",
      }],
    });

    expect(store.messages).toHaveLength(1);
    expect(store.messages[0]).toMatchObject({ id: "message-live", type: "score" });
    expect(store.messages[0]!.text).toContain("当前局外积分为 1920");
  });

  it("records the authoritative active room and clears stale connection errors", () => {
	const store = useGameStore();
	store.connectionError = "你已离开或被移出这个房间";
	store.acceptSnapshot({
	  ...structuredClone(emptySnapshot),
	  roomId: "room-target",
	  roomCode: "RF-TARG",
	  roomName: "目标房间",
	  players: [{
		id: "me", name: "你", initials: "你", seat: 1, tablePoints: 1000, accountPoints: 1000,
		streetCommitted: 0, status: "active", isDealer: false, isSpeaking: false,
		isCurrentActor: false, isLocal: true, isReady: false, isMuted: false,
	  }],
	});

	expect(store.activeRoomId).toBe("room-target");
	expect(store.connectionError).toBe("");
  });
});
