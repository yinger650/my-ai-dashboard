import type { BoardService, LogEntry, PinnedLog } from "../types";
import { dropServiceSuffixDupes, visibleHostServices } from "./host-services";

const CARD_SERVICE_ORDER = [
  "host-inspect",
  "nginx",
  "docker",
  "cron",
  "board-server",
  "sshd",
  "board-client",
];

export const DEFAULT_CARD_PINS = [
  { key: "host-listen", name: "监听端口" },
  { key: "nginx", name: "Nginx" },
  { key: "docker", name: "Docker" },
  { key: "cron", name: "Cron" },
] as const;

const CARD_PIN_KEYS = new Set(DEFAULT_CARD_PINS.map((p) => p.key));

export function compactCardServices(
  services: BoardService[],
  host?: { kind?: string | null; machineLastSeenAt?: string | null },
): BoardService[] {
  const filtered = dropServiceSuffixDupes(visibleHostServices(services, host ?? {}));
  const rank = (key: string) => {
    const i = CARD_SERVICE_ORDER.indexOf(key);
    return i === -1 ? 100 : i;
  };
  return [...filtered].sort((a, b) => {
    const d = rank(a.service_key) - rank(b.service_key);
    if (d !== 0) return d;
    return a.name.localeCompare(b.name, "zh");
  });
}

export function isCardNoiseLog(l: LogEntry): boolean {
  return l.source === "cron" || l.service_key === "cron";
}

export function compactCardPins(pins: PinnedLog[], extraKeys: string[] = []): PinnedLog[] {
  const extra = new Set(extraKeys);
  return pins.filter((p) => !p.service_key || CARD_PIN_KEYS.has(p.service_key) || extra.has(p.service_key));
}
