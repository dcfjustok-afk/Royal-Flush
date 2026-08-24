import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import QuickMessagePanel from "./QuickMessagePanel.vue";
import RoomManagementPanel from "./RoomManagementPanel.vue";
import { emptySnapshot } from "@/data/empty";
import { useGameStore } from "@/stores/game";

describe("room controls", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.restoreAllMocks();
  });

  it("sends a real remove-player command for another seated player", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const store = useGameStore();
    store.acceptSnapshot({
      ...structuredClone(emptySnapshot),
      roomId: "room-controls",
      ownerId: "player-owner",
      players: [
        { id: "player-owner", name: "房主甲", initials: "甲", seat: 0, tablePoints: 1000, accountPoints: 1000, streetCommitted: 0, status: "active", isDealer: true, isSpeaking: false, isCurrentActor: false, isLocal: true },
        { id: "player-guest", name: "玩家乙", initials: "乙", seat: 1, tablePoints: 1000, accountPoints: 1000, streetCommitted: 0, status: "active", isDealer: false, isSpeaking: false, isCurrentActor: false, isLocal: false },
      ],
    });
    const sendCommand = vi.spyOn(store, "sendCommand").mockResolvedValue(undefined as never);
    const wrapper = mount(RoomManagementPanel);

    await wrapper.get('[aria-label="移出 玩家乙"]').trigger("click");

    expect(sendCommand).toHaveBeenCalledWith("room.remove_player", { userId: "player-guest" });
    expect(wrapper.text()).toContain("玩家乙 已被移出房间");
  });

  it("sends only one of the fixed quick messages", async () => {
    const store = useGameStore();
    const sendCommand = vi.spyOn(store, "sendCommand").mockResolvedValue(undefined as never);
    const wrapper = mount(QuickMessagePanel);
    const button = wrapper.findAll("button").find((candidate) => candidate.text() === "好牌");

    await button!.trigger("click");

    expect(sendCommand).toHaveBeenCalledWith("room.quick_message", { message: "好牌" });
    expect(wrapper.text()).toContain("已发送：好牌");
  });
});
