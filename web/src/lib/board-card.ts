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

const CARD_PIN_KEYS = new Set(["host-listen", "nginx", "docker", "cron"]);

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

export function compactCardPins(pins: PinnedLog[]): PinnedLog[] {
  return pins.filter((p) => !p.service_key || CARD_PIN_KEYS.has(p.service_key));
}
