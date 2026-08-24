import { describe, expect, it } from "vitest";
import { isPoliteVoicePeer } from "./voice";

describe("voice peer negotiation roles", () => {
  it("assigns exactly one polite peer for every pair", () => {
    expect(isPoliteVoicePeer("user-a", "user-b")).toBe(false);
    expect(isPoliteVoicePeer("user-b", "user-a")).toBe(true);
  });

  it("keeps the role stable when users reconnect", () => {
    const first = isPoliteVoicePeer("player-20", "player-03");
    expect(isPoliteVoicePeer("player-20", "player-03")).toBe(first);
    expect(first).not.toBe(isPoliteVoicePeer("player-03", "player-20"));
  });
});
