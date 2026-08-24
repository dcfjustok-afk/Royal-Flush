import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it } from "vitest";
import { demoSnapshot } from "@/data/demo";
import { reconnectDelay, useGameStore } from "./game";

describe("game store realtime recovery", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("backs off quickly while keeping every retry within the recovery ceiling", () => {
    expect([0, 1, 2, 3, 8].map(reconnectDelay)).toEqual([250, 500, 1000, 2000, 2000]);
  });

  it("replaces demo broadcasts with persisted messages from the authoritative snapshot", () => {
    const store = useGameStore();
    store.acceptSnapshot({
      ...structuredClone(demoSnapshot),
      messages: [{
        id: "message-live",
        type: "score",
        text: "许栗自行增加了 1000 积分，当前局外积分为 1920",
        createdAt: "2026-08-24T14:08:09Z",
      }],
    });

    expect(store.messages).toHaveLength(1);
    expect(store.messages[0]).toMatchObject({ id: "message-live", type: "score" });
    expect(store.messages[0]!.text).toContain("当前局外积分为 1920");
  });
});
