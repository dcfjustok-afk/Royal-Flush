import { flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import QuickMessagePanel from "./QuickMessagePanel.vue";
import RoomManagementPanel from "./RoomManagementPanel.vue";
import { useGameStore } from "@/stores/game";

describe("room controls", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.restoreAllMocks();
  });

  it("lets the owner remove another player after confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const store = useGameStore();
    const wrapper = mount(RoomManagementPanel);

    await wrapper.get('[aria-label="移出 阿桥"]').trigger("click");
    await flushPromises();

    expect(store.snapshot.players.some((player) => player.name === "阿桥")).toBe(false);
    expect(wrapper.text()).toContain("阿桥 已被移出房间");
  });

  it("sends only one of the fixed quick messages", async () => {
    const store = useGameStore();
    const wrapper = mount(QuickMessagePanel);
    const button = wrapper.findAll("button").find((candidate) => candidate.text() === "好牌");

    await button!.trigger("click");
    await flushPromises();

    expect(store.messages[0]?.text).toBe("你：好牌");
    expect(wrapper.text()).toContain("已发送：好牌");
  });
});
