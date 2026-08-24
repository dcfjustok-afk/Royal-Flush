const baseUrl = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";
export const apiMode = import.meta.env.VITE_USE_API === "true";

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
};
