import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";

describe("global score reset", () => {
  afterEach(() => vi.useRealTimers());

  it("requires a reason and the exact confirmation phrase", async () => {
    vi.useFakeTimers();
    const wrapper = mount(App);
    const open = wrapper.findAll("button").find((button) => button.text().includes("重置全站积分"));
    expect(open).toBeDefined();
    await open!.trigger("click");

    const confirm = wrapper.get(".reset-dialog .danger-button");
    expect(confirm.attributes("disabled")).toBeDefined();
    await wrapper.get(".reset-dialog textarea").setValue("季度积分归零");
    await wrapper.get(".reset-dialog input").setValue("RESET ALL SCORE");
    expect(confirm.attributes("disabled")).toBeDefined();
    await wrapper.get(".reset-dialog input").setValue("RESET ALL SCORES");
    expect(confirm.attributes("disabled")).toBeUndefined();

    await confirm.trigger("click");
    expect(wrapper.text()).toContain("重置已完成");
    expect(wrapper.text()).toContain("Epoch 5");
  });
});

