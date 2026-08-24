const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";
export const apiMode = import.meta.env.VITE_USE_API === "true";

export interface OperationsUser {
  id: string;
  phone: string;
  nickname: string;
  balance: number;
  activeRoomId?: string;
  banned: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AdminRoom {
  id: string;
  code: string;
  name: string;
  ownerId: string;
  ownerName: string;
  players: number;
  onlinePlayers: number;
  maxPlayers: number;
  blindPreset: string;
  handNumber: number;
  voiceEnabled: boolean;
  status: "waiting" | "playing" | "ended";
  version: number;
  createdAt: string;
}

export interface Report {
  id: string;
  reporterId: string;
  roomId?: string;
  subjectUserId?: string;
  category: "conduct" | "voice" | "technical" | "other";
  detail: string;
  status: "open" | "reviewing" | "resolved" | "dismissed";
  handledBy?: string;
  handledAt?: string;
  createdAt: string;
}

export interface AdminAudit {
  id: string;
  administratorId: string;
  action: string;
  targetType: string;
  targetId?: string;
  reason: string;
  requestId: string;
  metadata: Record<string, unknown>;
  createdAt: string;
}

export interface AdminRoomSnapshot {
  roomId: string;
  roomCode: string;
  roomName: string;
  ownerId: string;
  version: number;
  handNumber: number;
  street: string;
  pot: number;
  players: Array<{
    id: string;
    name: string;
    seat: number;
    tablePoints: number;
    accountPoints: number;
    status: string;
    isReady: boolean;
    isMuted: boolean;
  }>;
  config: { blindPreset: string; actionSeconds: number; voiceEnabled: boolean; chipDenominations: number[] };
  messages: Array<{ id: string; text: string; createdAt: string }>;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  if (import.meta.env.DEV) {
    headers.set("X-User-ID", "local-admin");
    headers.set("X-Admin", "true");
  }
  const response = await fetch(`${baseUrl}${path}`, {
    credentials: "include",
    ...init,
    headers,
  });
  if (!response.ok) {
    const problem = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(problem?.message ?? `请求失败 (${response.status})`);
  }
  return response.json() as Promise<T>;
}

export const adminApi = {
  epochs: () => request<{ epochs: Array<{ id: number; reason: string; administrator: string; createdAt: string }> }>("/admin/score-epochs"),
  resetScores: (reason: string) => request<{ epoch: number; baseScore: 1000; duplicate: boolean }>("/admin/score-resets", {
    method: "POST",
    body: JSON.stringify({ reason, confirmation: "RESET ALL SCORES", requestId: crypto.randomUUID() }),
  }),
  users: (query = "") => request<{ users: OperationsUser[] }>(`/admin/users?q=${encodeURIComponent(query)}&limit=200`),
  setUserBanned: (userId: string, banned: boolean, reason: string) => request<{ user: OperationsUser; duplicate: boolean }>(`/admin/users/${encodeURIComponent(userId)}/ban-actions`, {
    method: "POST",
    body: JSON.stringify({ banned, reason, requestId: crypto.randomUUID() }),
  }),
  rooms: () => request<{ rooms: AdminRoom[] }>("/admin/rooms"),
  room: (roomId: string) => request<AdminRoomSnapshot>(`/admin/rooms/${encodeURIComponent(roomId)}`),
  reports: (status = "") => request<{ reports: Report[] }>(`/admin/reports?status=${encodeURIComponent(status)}&limit=200`),
  resolveReport: (reportId: string, status: "resolved" | "dismissed", reason: string) => request<{ report: Report; duplicate: boolean }>(`/admin/reports/${encodeURIComponent(reportId)}/resolution`, {
    method: "POST",
    body: JSON.stringify({ status, reason, requestId: crypto.randomUUID() }),
  }),
  audits: () => request<{ audits: AdminAudit[] }>("/admin/audit-log?limit=200"),
};
