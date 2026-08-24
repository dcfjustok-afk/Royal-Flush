import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

describe("operations console", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("shows real empty state when the operations API is not configured", async () => {
    const { default: App } = await import("./App.vue");
    const wrapper = mount(App);

    expect(wrapper.text()).toContain("当前没有活跃房间");
    expect(wrapper.text()).toContain("当前 Epoch--");
    expect(wrapper.text()).toContain("暂无管理员操作");
    expect(wrapper.text()).not.toContain("林度");
    expect(wrapper.text()).not.toContain("RF-2806");
  });

  it("renders the epoch returned by the real reset API", async () => {
    vi.stubEnv("VITE_USE_API", "true");
    const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), {
      status,
      headers: { "Content-Type": "application/json" },
    });
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/me")) {
        return Promise.resolve(json({
          user: { id: "operator-1", phone: "13800138000", nickname: "夜班运营", permissions: { "admin:read": true } },
          balance: 1000,
        }));
      }
      if (url.includes("/admin/score-epochs")) return Promise.resolve(json({ epochs: [{ id: 1, reason: "initial", administrator: "system", createdAt: new Date().toISOString() }] }));
      if (url.includes("/admin/rooms")) return Promise.resolve(json({ rooms: [] }));
      if (url.includes("/admin/users")) return Promise.resolve(json({ users: [] }));
      if (url.includes("/admin/reports")) return Promise.resolve(json({ reports: [] }));
      if (url.includes("/admin/audit-log")) return Promise.resolve(json({ audits: [] }));
      if (url.endsWith("/admin/score-resets")) {
        expect(init?.method).toBe("POST");
        return Promise.resolve(json({ epoch: 2, baseScore: 1000, duplicate: false }));
      }
      return Promise.resolve(json({ code: "not_found" }, 404));
    });
    vi.stubGlobal("fetch", fetchMock);

    const { default: App } = await import("./App.vue");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain("夜班运营");
    const open = wrapper.findAll("button").find((button) => button.text().includes("重置全站积分"));
    expect(open).toBeDefined();
    await open!.trigger("click");
    await wrapper.get(".reset-dialog textarea").setValue("开始新的积分周期");
    await wrapper.get(".reset-dialog input").setValue("RESET ALL SCORES");
    await wrapper.get(".reset-dialog .danger-button").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("重置已完成");
    expect(wrapper.text()).toContain("Epoch 2");
  });
});
