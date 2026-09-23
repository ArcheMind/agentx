import { spawn } from "node:child_process";

const [secondsText, command, ...args] = process.argv.slice(2);
const seconds = Number(secondsText);
if (!Number.isFinite(seconds) || seconds <= 0 || !command) {
  console.error("usage: run-with-timeout.mjs <seconds> <command> [args...]");
  process.exit(2);
}

const detached = process.platform !== "win32";
const child = spawn(command, args, { detached, stdio: "inherit" });
let timedOut = false;
let forceTimer;
const kill = (signal) => {
  try {
    if (detached) process.kill(-child.pid, signal);
    else child.kill(signal);
  } catch (error) {
    if (error.code !== "ESRCH") throw error;
  }
};
const timer = setTimeout(() => {
  timedOut = true;
  console.error(`command timed out after ${seconds} seconds: ${command}`);
  kill("SIGTERM");
  forceTimer = setTimeout(() => kill("SIGKILL"), 5000);
}, seconds * 1000);

child.on("error", (error) => {
  clearTimeout(timer);
  console.error(error.message);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  clearTimeout(timer);
  clearTimeout(forceTimer);
  if (timedOut) process.exit(124);
  if (signal) process.exit(1);
  process.exit(code ?? 1);
});
