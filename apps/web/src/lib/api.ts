import type { RoomConfig, RoomEvent, ScoreAdditionRequest, ScoreLedgerEntry, ScoreResetRequest, TableSnapshot } from "@royal-flush/contracts";

const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";
export const apiMode = import.meta.env.VITE_USE_API === "true";

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export type ReportCategory = "conduct" | "voice" | "technical" | "other";

export interface ReportRecord {
  id: string;
  reporterId: string;
  roomId?: string;
  subjectUserId?: string;
  category: ReportCategory;
  detail: string;
  status: "open" | "reviewing" | "resolved" | "dismissed";
  handledBy?: string;
  handledAt?: string;
  createdAt: string;
}

export interface CreateReportRequest {
  roomId?: string;
  subjectUserId?: string;
  category: ReportCategory;
  detail: string;
  requestId: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
    ...init,
  });

  if (!response.ok) {
    const problem = (await response.json().catch(() => null)) as { code?: string; message?: string } | null;
    throw new ApiError(problem?.message ?? `请求失败 (${response.status})`, response.status, problem?.code ?? "request_failed");
  }
  return response.json() as Promise<T>;
}

export const api = {
  health: () => request<{ status: string }>("/health"),
  requestOtp: (phone: string) => request<{ expiresAt: string; expiresIn: number; devCode?: string }>("/auth/otp/request", { method: "POST", body: JSON.stringify({ phone }) }),
  verifyOtp: (phone: string, code: string, nickname?: string) => request<{ user: { id: string; nickname: string }; balance: number }>("/auth/otp/verify", { method: "POST", body: JSON.stringify({ phone, code, nickname }) }),
  me: () => request<{ user: { id: string; nickname: string }; balance: number; activeRoomId: string }>("/me"),
  addScore: (body: ScoreAdditionRequest) => request<{ balance: number; entry: ScoreLedgerEntry }>("/me/score-additions", { method: "POST", body: JSON.stringify(body) }),
  scoreLedger: () => request<{ balance: number; entries: ScoreLedgerEntry[] }>("/me/score-ledger"),
  publicRoom: (idOrCode: string) => request<{ id: string; code: string; name: string; ownerId: string; ownerName: string; onlinePlayers: number; maxPlayers: number; occupiedSeats: number[]; config: RoomConfig }>(`/rooms/${encodeURIComponent(idOrCode)}/public`),
  createRoom: (config: RoomConfig) => request<{ id: string; code: string; config: RoomConfig; snapshot: TableSnapshot }>("/rooms", { method: "POST", body: JSON.stringify(config) }),
  joinRoom: (idOrCode: string, seat: number) => request<TableSnapshot>(`/rooms/${encodeURIComponent(idOrCode)}/join`, { method: "POST", body: JSON.stringify({ seat }) }),
  roomSnapshot: (roomId: string) => request<TableSnapshot>(`/rooms/${roomId}/snapshot`),
  roomCommand: (roomId: string, body: { type: string; requestId: string; expectedVersion: number; payload: Record<string, unknown> }) => request<{ event: RoomEvent; duplicate: boolean }>(`/rooms/${roomId}/commands`, { method: "POST", body: JSON.stringify(body) }),
  voiceToken: (roomId: string) => request<{ enabled: boolean; url?: string; accessToken?: string; expiresAt?: string; reason?: string }>(`/rooms/${roomId}/voice-token`, { method: "POST", body: "{}" }),
  createReport: (body: CreateReportRequest) => request<{ report: ReportRecord; duplicate: boolean }>("/reports", { method: "POST", body: JSON.stringify(body) }),
  resetScores: (body: ScoreResetRequest) => request<{ epoch: number; baseScore: number }>("/admin/score-resets", { method: "POST", body: JSON.stringify(body) }),
  webSocketUrl: (roomId: string) => {
    const configured = import.meta.env.VITE_WS_BASE_URL as string | undefined;
    if (configured) return `${configured.replace(/\/$/, "")}/rooms/${roomId}/events`;
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${location.host}${baseUrl}/rooms/${roomId}/events`;
  },
};
