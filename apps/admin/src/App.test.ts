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

  it("requires an auditable reason before changing a user ban", async () => {
    const wrapper = mount(App);
    const usersNav = wrapper.findAll("nav button").find((button) => button.text().includes("用户与积分"));
    await usersNav!.trigger("click");
    const unban = wrapper.find('button[title="解除封禁"]');
    expect(unban.exists()).toBe(true);
    await unban.trigger("click");
    const confirm = wrapper.get(".action-dialog .confirm-button");
    expect(confirm.attributes("disabled")).toBeDefined();
    await wrapper.get(".action-dialog textarea").setValue("复核完成");
    expect(confirm.attributes("disabled")).toBeUndefined();
    await confirm.trigger("click");
    expect(wrapper.text()).toContain("林度");
    expect(wrapper.find('button[title="封禁用户"]').exists()).toBe(true);
  });

  it("handles a report without losing the queue history", async () => {
    const wrapper = mount(App);
    const reportsNav = wrapper.findAll("nav button").find((button) => button.text().includes("举报处理"));
    await reportsNav!.trigger("click");
    const handle = wrapper.findAll(".report-list button").find((button) => button.text().includes("处理"));
    await handle!.trigger("click");
    await wrapper.get(".action-dialog textarea").setValue("已完成核查");
    await wrapper.get(".action-dialog .confirm-button").trigger("click");
    expect(wrapper.text()).toContain("已解决");
    expect(wrapper.findAll(".report-list article")).toHaveLength(2);
  });
});
