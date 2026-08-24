import { flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useGameStore } from "@/stores/game";
import ScoreAddPanel from "./ScoreAddPanel.vue";

describe("ScoreAddPanel", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("submits a rapid double click only once", async () => {
    const store = useGameStore();
    let resolveRequest!: () => void;
    const request = new Promise<void>((resolve) => { resolveRequest = resolve; });
    const addPoints = vi.spyOn(store, "addAccountPoints").mockImplementation(() => request);
    const wrapper = mount(ScoreAddPanel);

    await wrapper.find("form").trigger("submit");
    await wrapper.find("form").trigger("submit");

    expect(addPoints).toHaveBeenCalledTimes(1);
    expect(wrapper.get("button[type='submit']").attributes("disabled")).toBeDefined();
    expect(wrapper.get("button[type='submit']").text()).toContain("增加中");

    resolveRequest();
    await flushPromises();
    expect(wrapper.get("button[type='submit']").attributes("disabled")).toBeUndefined();
    expect(wrapper.text()).toContain("已增加 500 积分");
  });
});
