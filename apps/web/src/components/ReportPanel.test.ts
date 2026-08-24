import { mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it } from "vitest";
import ReportPanel from "./ReportPanel.vue";

describe("ReportPanel", () => {
  beforeEach(() => setActivePinia(createPinia()));

  it("keeps reports unavailable when the real service is offline", async () => {
    const wrapper = mount(ReportPanel);
    await wrapper.get(".report-panel-toggle").trigger("click");
    await wrapper.get("textarea").setValue("持续干扰语音");

    const submit = wrapper.get('button[type="submit"]');
    expect(submit.attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("服务暂时不可用");
    expect((wrapper.get("textarea").element as HTMLTextAreaElement).value).toBe("持续干扰语音");
    expect(wrapper.text()).not.toContain("举报已登记");
  });
});
