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
});
