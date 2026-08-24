import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

describe("admin authentication", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("shows the login gate when no session exists", async () => {
    vi.stubEnv("VITE_USE_API", "true");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: "authentication_required",
      message: "请先完成手机号验证码登录",
    }), { status: 401, headers: { "Content-Type": "application/json" } })));
    const { default: App } = await import("./App.vue");

    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain("运营身份验证");
    expect(wrapper.find(".admin-shell").exists()).toBe(false);
    expect(wrapper.find('input[autocomplete="tel"]').exists()).toBe(true);
  });

  it("loads operations data only after an authorized OTP login", async () => {
    vi.stubEnv("VITE_USE_API", "true");
    let meCalls = 0;
    const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
    const fetchMock = vi.fn().mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/me")) {
        meCalls += 1;
        if (meCalls === 1) return Promise.resolve(json({ code: "authentication_required", message: "请先登录" }, 401));
        return Promise.resolve(json({ user: { id: "admin-1", phone: "13800138000", nickname: "夜班运营", permissions: { "admin:read": true } }, balance: 1000 }));
      }
      if (url.endsWith("/auth/otp/request")) return Promise.resolve(json({ expiresAt: new Date().toISOString(), expiresIn: 300, devCode: "123456" }));
      if (url.endsWith("/auth/otp/verify")) return Promise.resolve(json({ user: { id: "admin-1", phone: "13800138000", nickname: "夜班运营", permissions: {} }, balance: 1000 }));
      if (url.includes("/admin/score-epochs")) return Promise.resolve(json({ epochs: [{ id: 7, reason: "初始化", administrator: "admin-1", createdAt: new Date().toISOString() }] }));
      if (url.includes("/admin/rooms")) return Promise.resolve(json({ rooms: null }));
      if (url.includes("/admin/users")) return Promise.resolve(json({ users: null }));
      if (url.includes("/admin/reports")) return Promise.resolve(json({ reports: null }));
      if (url.includes("/admin/audit-log")) return Promise.resolve(json({ audits: null }));
      return Promise.resolve(json({ code: "not_found" }, 404));
    });
    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("./App.vue");
    const wrapper = mount(App);
    await flushPromises();

    await wrapper.get('input[autocomplete="tel"]').setValue("13800138000");
    await wrapper.get(".admin-login").trigger("submit");
    await flushPromises();
    expect(wrapper.text()).toContain("预览环境验证码");

    await wrapper.get('input[autocomplete="one-time-code"]').setValue("123456");
    await wrapper.get(".admin-login").trigger("submit");
    await flushPromises();

    expect(wrapper.find(".admin-shell").exists()).toBe(true);
    expect(wrapper.text()).toContain("夜班运营");
    expect(wrapper.text()).toContain("Epoch");
    expect(meCalls).toBe(2);
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes("/admin/users"))).toBe(true);
  });
});
