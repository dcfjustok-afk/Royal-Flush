import { afterEach, describe, expect, it, vi } from "vitest";
import { adminApi } from "./api";

describe("admin score API", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("sends an auditable reset request with development credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ epoch: 5, baseScore: 1000, duplicate: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(adminApi.resetScores("季度积分归零")).resolves.toMatchObject({ epoch: 5, baseScore: 1000 });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/admin/score-resets");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(new Headers(init.headers).get("X-Admin")).toBe("true");
    expect(JSON.parse(String(init.body))).toMatchObject({
      reason: "季度积分归零",
      confirmation: "RESET ALL SCORES",
      requestId: expect.any(String),
    });
  });

  it("sends idempotent moderation and report resolution commands", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ user: { id: "u1", banned: true }, duplicate: false }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ report: { id: "r1", status: "resolved" }, duplicate: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await adminApi.setUserBanned("u1", true, "违反房间秩序");
    await adminApi.resolveReport("r1", "resolved", "已完成核查");

    const [banURL, banInit] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(banURL).toBe("/api/v1/admin/users/u1/ban-actions");
    expect(JSON.parse(String(banInit.body))).toMatchObject({ banned: true, reason: "违反房间秩序", requestId: expect.any(String) });
    const [reportURL, reportInit] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(reportURL).toBe("/api/v1/admin/reports/r1/resolution");
    expect(JSON.parse(String(reportInit.body))).toMatchObject({ status: "resolved", reason: "已完成核查", requestId: expect.any(String) });
  });
});
