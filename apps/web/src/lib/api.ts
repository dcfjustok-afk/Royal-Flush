import type { RoomConfig, ScoreAdditionRequest, ScoreLedgerEntry, ScoreResetRequest, TableSnapshot } from "@royal-flush/contracts";

const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
    ...init,
  });

  if (!response.ok) {
    const problem = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new Error(problem?.message ?? `请求失败 (${response.status})`);
  }
  return response.json() as Promise<T>;
}

export const api = {
  health: () => request<{ status: string }>("/health"),
  addScore: (body: ScoreAdditionRequest) => request<{ balance: number; entry: ScoreLedgerEntry }>("/me/score-additions", { method: "POST", body: JSON.stringify(body) }),
  scoreLedger: () => request<{ balance: number; entries: ScoreLedgerEntry[] }>("/me/score-ledger"),
  createRoom: (config: RoomConfig) => request<{ id: string; code: string; config: RoomConfig }>("/rooms", { method: "POST", body: JSON.stringify(config) }),
  roomSnapshot: (roomId: string) => request<TableSnapshot>(`/rooms/${roomId}/snapshot`),
  resetScores: (body: ScoreResetRequest) => request<{ epoch: number; baseScore: number }>("/admin/score-resets", { method: "POST", body: JSON.stringify(body) }),
};

