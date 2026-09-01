import { describe, expect, it } from "vitest";
import {
  dropServiceSuffixDupes,
  isHostMachineKind,
  visibleHostServices,
} from "./host-services";

function svc(
  key: string,
  extra: Partial<{ current_state: string; last_seen_at: string | null; name: string }> = {},
) {
  return {
    service_key: key,
    name: extra.name ?? key,
    current_state: extra.current_state ?? "running",
    last_seen_at: extra.last_seen_at === undefined ? "2026-08-27T11:38:00.000Z" : extra.last_seen_at,
  };
}

const machineSeen = "2026-08-27T11:38:49.000Z";
const hostOpts = { kind: "vm" as const, machineLastSeenAt: machineSeen };

describe("isHostMachineKind", () => {
  it("treats physical/vm/container_host as hosts", () => {
    expect(isHostMachineKind("vm")).toBe(true);
    expect(isHostMachineKind("physical")).toBe(true);
    expect(isHostMachineKind("container_host")).toBe(true);
  });

  it("does not treat virtual or empty as hosts", () => {
    expect(isHostMachineKind("virtual")).toBe(false);
    expect(isHostMachineKind("")).toBe(false);
    expect(isHostMachineKind(undefined)).toBe(false);
  });
});

describe("visibleHostServices", () => {
  it("hides leftover .service keys that stopped reporting 21h ago", () => {
    const rows = visibleHostServices(
      [
        svc("board-client"),
        svc("board-client.service", {
          name: "AgentBoard Personal board-client",
          last_seen_at: "2026-08-26T14:20:38.000Z",
        }),
      ],
      hostOpts,
    );
    expect(rows.map((s) => s.service_key)).toEqual(["board-client"]);
  });

  it("keeps a live board-client that is still being collected", () => {
    const rows = visibleHostServices([svc("board-client")], hostOpts);
    expect(rows.map((s) => s.service_key)).toEqual(["board-client"]);
  });

  it("keeps failed nginx that is still being collected", () => {
    const rows = visibleHostServices(
      [svc("nginx", { current_state: "failed", last_seen_at: machineSeen })],
      hostOpts,
    );
    expect(rows.map((s) => s.service_key)).toEqual(["nginx"]);
  });

  it("hides unknown docker (not installed)", () => {
    const rows = visibleHostServices(
      [svc("docker", { current_state: "unknown", last_seen_at: machineSeen })],
      hostOpts,
    );
    expect(rows).toEqual([]);
  });

  it("hides stopped units even if last_seen is fresh", () => {
    const rows = visibleHostServices(
      [svc("sshd", { current_state: "stopped", last_seen_at: machineSeen })],
      hostOpts,
    );
    expect(rows).toEqual([]);
  });

  it("hides foo.service when foo exists and both are live", () => {
    const rows = visibleHostServices(
      [
        svc("board-server", { name: "Board Server" }),
        svc("board-server.service", { name: "AgentBoard Personal board-server" }),
      ],
      hostOpts,
    );
    expect(rows.map((s) => s.service_key)).toEqual(["board-server"]);
  });

  it("does not filter virtual machines", () => {
    const leftover = svc("board-client.service", {
      last_seen_at: "2026-08-26T14:20:38.000Z",
    });
    const unknown = svc("docker", { current_state: "unknown" });
    const rows = visibleHostServices([leftover, unknown], {
      kind: "virtual",
      machineLastSeenAt: machineSeen,
    });
    expect(rows.map((s) => s.service_key)).toEqual(["board-client.service", "docker"]);
  });

  it("hides host services with a null last_seen", () => {
    const rows = visibleHostServices([svc("cron", { last_seen_at: null })], hostOpts);
    expect(rows).toEqual([]);
  });
});

describe("dropServiceSuffixDupes", () => {
  it("drops .service when the bare key exists", () => {
    const rows = dropServiceSuffixDupes([svc("nginx.service"), svc("nginx"), svc("sshd.service")]);
    expect(rows.map((s) => s.service_key)).toEqual(["nginx", "sshd.service"]);
  });
});
