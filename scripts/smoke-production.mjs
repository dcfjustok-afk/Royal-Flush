const webUrl = new URL(process.env.WEB_URL ?? "https://royal-flush.zeabur.app/");
const adminUrl = new URL(process.env.ADMIN_URL ?? "https://royal-flush-admin.zeabur.app/");
const attempts = Number.parseInt(process.env.SMOKE_ATTEMPTS ?? "12", 10);
const delayMs = Number.parseInt(process.env.SMOKE_DELAY_MS ?? "5000", 10);

const targets = [
  { name: "player web", url: webUrl, validate: (body) => body.includes("Royal Flush") },
  { name: "server readiness", url: new URL("/api/v1/ready", webUrl), validate: (body) => JSON.parse(body).status === "ready" },
  { name: "admin web", url: adminUrl, validate: (body) => body.includes("Royal Flush") },
];

const pause = (duration) => new Promise((resolve) => setTimeout(resolve, duration));

async function probe(target) {
  const response = await fetch(target.url, {
    headers: { "User-Agent": "Royal-Flush-production-smoke/1.0" },
    redirect: "follow",
    signal: AbortSignal.timeout(10_000),
  });
  const body = await response.text();
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  if (!target.validate(body)) throw new Error("响应内容未通过校验");
  return { name: target.name, url: target.url.toString(), status: response.status };
}

async function probeWithRetry(target) {
  let lastError;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const result = await probe(target);
      console.log(JSON.stringify({ ...result, attempt }));
      return;
    } catch (error) {
      lastError = error;
      console.warn(`${target.name} 第 ${attempt}/${attempts} 次检查失败：${error instanceof Error ? error.message : String(error)}`);
      if (attempt < attempts) await pause(delayMs);
    }
  }
  throw new Error(`${target.name} 在 ${attempts} 次检查后仍不可用`, { cause: lastError });
}

await Promise.all(targets.map(probeWithRetry));
console.log("Production smoke checks passed.");
