import { afterEach, describe, expect, it, vi } from "vitest";
import { api, isSessionRevokedClose } from "./api";

describe("player API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("submits an idempotent room report", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      report: { id: "report-1", reporterId: "me", roomId: "room-1", subjectUserId: "p2", category: "voice", detail: "持续干扰语音", status: "open", createdAt: "2026-08-24T08:00:00Z" },
      duplicate: false,
    }), { status: 201, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.createReport({ roomId: "room-1", subjectUserId: "p2", category: "voice", detail: "持续干扰语音", requestId: "report-request-1" });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/reports");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ roomId: "room-1", subjectUserId: "p2", category: "voice", detail: "持续干扰语音", requestId: "report-request-1" });
  });

  it("recognizes realtime connections revoked by logout or ban", () => {
    expect(isSessionRevokedClose(1008)).toBe(true);
    expect(isSessionRevokedClose(1000)).toBe(false);
    expect(isSessionRevokedClose(1006)).toBe(false);
  });
});
