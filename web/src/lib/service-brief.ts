const FUNCTION_BY_KEY: Record<string, string> = {
  "host-inspect": "主机巡检：把 Docker、cron、nginx、端口等快照投影到看板，自身只报存活。",
  "host-listen": "汇总本机当前监听端口。",
  nginx: "反向代理与静态站点，把对外域名转到本机端口。",
  docker: "容器运行时，汇总本机容器状态。",
  cron: "系统定时任务（crontab）的日程与最近执行。",
  sshd: "SSH 远程登录服务。",
  "board-client": "本机采集客户端，上报指标、巡检与心跳。",
  "board-server": "AgentBoard 看板服务端。",
  "cursor-agent": "扫描 Cursor Agent 会话并做启发式日志总结。",
  cursor: "Cursor Agent 任务上报。",
  codex: "Codex Agent 任务上报。",
  openclaw: "OpenClaw Agent 任务上报。",
};

const TYPE_HINT: Record<string, string> = {
  daemon: "常驻进程",
  scheduled: "定时任务",
  job: "一次性任务",
  agent: "Agent 任务",
  virtual: "虚拟聚合服务",
};

const STATE_ZH: Record<string, string> = {
  running: "运行中",
  stopped: "已停止",
  failed: "失败",
  starting: "启动中",
  stopping: "停止中",
  alive: "存活",
  unknown: "未知",
  inactive: "未运行",
  active: "运行中",
};

const SEV_ZH: Record<string, string> = {
  normal: "正常",
  info: "信息",
  warning: "告警",
  error: "异常",
  unknown: "未知",
};

export interface ServiceBriefInput {
  service_key: string;
  name: string;
  type: string;
  description?: string | null;
  current_state: string;
  state_summary: string;
  severity: string;
}

function bareKey(key: string): string {
  return key.endsWith(".service") ? key.slice(0, -".service".length) : key;
}

export function describeServiceFunction(s: ServiceBriefInput): string {
  const desc = s.description?.trim();
  if (desc) return desc.endsWith("。") ? desc : `${desc}。`;

  const key = bareKey(s.service_key);
  if (FUNCTION_BY_KEY[key]) return FUNCTION_BY_KEY[key];
  if (FUNCTION_BY_KEY[s.service_key]) return FUNCTION_BY_KEY[s.service_key];
  if (s.service_key.startsWith("site-")) return "从本机对公网 URL 做 HTTP 健康探测，不是站点自己的进程。";
  if (s.service_key.endsWith(".service")) return `systemd 单元「${s.name}」。`;
  const hint = TYPE_HINT[s.type];
  if (hint) return `${hint}「${s.name}」。`;
  return `服务「${s.name}」。`;
}

export function describeServiceStatus(s: ServiceBriefInput): string {
  const sev = SEV_ZH[s.severity] ?? s.severity;
  const state = STATE_ZH[s.current_state] ?? (s.current_state || "未知");
  const summary = s.state_summary?.trim();
  if (summary && summary !== s.current_state) {
    return `状态：${summary}（${sev}）。`;
  }
  return `状态：${state}（${sev}）。`;
}

export function describeService(s: ServiceBriefInput): string {
  return `${describeServiceFunction(s)} ${describeServiceStatus(s)}`;
}

export function servicePathLabel(path: string): string {
  return /^https?:\/\//i.test(path.trim()) ? "探测 URL" : "主进程";
}
