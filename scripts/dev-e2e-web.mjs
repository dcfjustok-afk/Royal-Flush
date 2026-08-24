import { spawn } from "node:child_process";

const npmArgs = ["run", "dev", "-w", "@royal-flush/web", "--", "--host", "127.0.0.1", "--port", "5175"];
const windows = process.platform === "win32";
const command = windows ? process.env.ComSpec ?? "cmd.exe" : "npm";
const args = windows ? ["/d", "/s", "/c", `npm ${npmArgs.join(" ")}`] : npmArgs;
const child = spawn(command, args, {
  env: { ...process.env, VITE_USE_API: "true" },
  stdio: "inherit",
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => child.kill(signal));
}

child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 0);
});
