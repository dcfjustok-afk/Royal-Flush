import { flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it } from "vitest";
import ReportPanel from "./ReportPanel.vue";

describe("ReportPanel", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("requires useful detail and resets the form after a demo submission", async () => {
    const wrapper = mount(ReportPanel);
    await wrapper.get(".report-panel-toggle").trigger("click");

    const submit = wrapper.get('button[type="submit"]');
    expect(submit.attributes("disabled")).toBeDefined();

    await wrapper.get("textarea").setValue("持续干扰语音");
    await wrapper.findAll("select")[0]!.setValue("voice");
    await wrapper.findAll("select")[1]!.setValue("p2");
    expect(submit.attributes("disabled")).toBeUndefined();

    await submit.trigger("submit");
    await flushPromises();

    expect(wrapper.text()).toContain("举报已登记");
    expect((wrapper.get("textarea").element as HTMLTextAreaElement).value).toBe("");
    expect((wrapper.findAll("select")[0]!.element as HTMLSelectElement).value).toBe("conduct");
  });
});
