export const MACHINE_KEY_PLACEHOLDER = "{Machine Key}";
export const BOARD_URL_PLACEHOLDER = "{Board URL}";
export const SKILL_REPO = "https://github.com/yinger650/my-ai-dashboard";
export const GITHUB_CLIENT_DOWNLOAD =
  "https://github.com/yinger650/my-ai-dashboard/releases/latest/download";

export const LINUX_ASSETS = [
  { id: "amd64", file: "board-client-linux-amd64", label: "Linux x86_64" },
  { id: "arm64", file: "board-client-linux-arm64", label: "Linux ARM64" },
] as const;

export function boardOrigin(publicUrl: string, fallback = ""): string {
  const raw = (publicUrl || fallback).trim();
  return raw.replace(/\/+$/, "");
}

/** One-shot line the user speaks to Cursor / Claude / Codex. */
export function agentInstallPrompt(boardUrl: string, machineKey: string): string {
  const url = boardUrl.trim() || BOARD_URL_PLACEHOLDER;
  const key = machineKey.trim() || MACHINE_KEY_PLACEHOLDER;
  return [
    `把 AgentBoard 装进当前项目，并立刻开始上报。`,
    `看板地址：${url}`,
    `项目 Machine Token：${key}`,
    `从 ${SKILL_REPO} 拉取 skills/agentboard-report（必须含 scripts/report.py）。把下面三行写入仓库根目录 .env（已 gitignore，禁止提交）：`,
    `AGENTBOARD_URL=${url}`,
    `AGENTBOARD_TOKEN=${key}`,
    `AGENTBOARD_PROVIDER=cursor`,
    `把 skills/agentboard-report/always-on.md 并入 AGENTS.md；Cursor 再写入 .cursor/rules；Claude Code 再写入 CLAUDE.md。装好后立刻 start「正在验证 AgentBoard 上报」，看板上出现任务后再 succeed。不要把 Token 提交进 git。`,
  ].join("\n");
}

/** Ask an on-box agent to install board-client on this Linux host. */
export function hostInstallPrompt(opts: {
  boardUrl: string;
  machineKey: string;
  machineCode: string;
  downloadAmd64: string;
  downloadArm64: string;
}): string {
  const url = opts.boardUrl.trim() || BOARD_URL_PLACEHOLDER;
  const key = opts.machineKey.trim() || MACHINE_KEY_PLACEHOLDER;
  const code = opts.machineCode.trim() || "home-server";
  return [
    `请在这台 Linux 上安装 AgentBoard 采集客户端 board-client（不要装 macOS / Windows，还在开发）。`,
    `上报地址：${url}`,
    `机器代号 machine.key：${code}`,
    `Machine Token：${key}`,
    `按架构下载：x86_64 → ${opts.downloadAmd64} ；ARM64 → ${opts.downloadArm64} 。`,
    `装到 /opt/agentboard/bin/board-client，Token 写入 /etc/agentboard/board-client.env（权限 600），client.yaml 里填 server.url 与 machine.key，然后 board-client run。`,
  ].join("\n");
}

export function clientDownloadUrl(origin: string, file: string, viaGithub = false): string {
  if (viaGithub) return `${GITHUB_CLIENT_DOWNLOAD}/${file}`;
  const base = boardOrigin(origin);
  if (!base) return `${GITHUB_CLIENT_DOWNLOAD}/${file}`;
  return `${base}/client-updates/${file}`;
}

export function hostEnvSnippet(machineKey: string): string {
  const key = machineKey.trim() || MACHINE_KEY_PLACEHOLDER;
  return `ABP_MACHINE_TOKEN=${key}`;
}

export function hostYamlSnippet(boardUrl: string, machineCode: string): string {
  const url = boardUrl.trim() || BOARD_URL_PLACEHOLDER;
  const code = machineCode.trim() || "home-server";
  return [
    "server:",
    `  url: "${url}"`,
    "machine:",
    `  key: "${code}"`,
    `  display_name: "${code}"`,
  ].join("\n");
}

export function agentEnvSnippet(boardUrl: string, machineKey: string): string {
  const url = boardUrl.trim() || BOARD_URL_PLACEHOLDER;
  const key = machineKey.trim() || MACHINE_KEY_PLACEHOLDER;
  return [
    `AGENTBOARD_URL=${url}`,
    `AGENTBOARD_TOKEN=${key}`,
    `AGENTBOARD_PROVIDER=cursor`,
  ].join("\n");
}
