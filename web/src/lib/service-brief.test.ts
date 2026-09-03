import { describe, expect, it } from "vitest";
import { describeService, describeServiceFunction, describeServiceStatus, servicePathLabel } from "./service-brief";

describe("describeServiceFunction", () => {
  it("uses stored description when present", () => {
    expect(
      describeServiceFunction({
        service_key: "nginx",
        name: "Nginx",
        type: "daemon",
        description: "自定义说明",
        current_state: "running",
        state_summary: "ok",
        severity: "normal",
      }),
    ).toBe("自定义说明。");
  });

  it("maps known keys and site probes", () => {
    expect(
      describeServiceFunction({
        service_key: "nginx",
        name: "Nginx",
        type: "daemon",
        description: "",
        current_state: "running",
        state_summary: "",
        severity: "normal",
      }),
    ).toMatch(/反向代理/);
    expect(
      describeServiceFunction({
        service_key: "site-board-yinger650-com",
        name: "board",
        type: "virtual",
        description: "",
        current_state: "running",
        state_summary: "",
        severity: "normal",
      }),
    ).toMatch(/不是站点自己的进程/);
  });
});

describe("describeServiceStatus", () => {
  it("prefers state_summary and severity", () => {
    expect(
      describeServiceStatus({
        service_key: "nginx",
        name: "Nginx",
        type: "daemon",
        current_state: "running",
        state_summary: "3 条生效反代",
        severity: "normal",
      }),
    ).toBe("状态：3 条生效反代（正常）。");
  });
});

describe("describeService", () => {
  it("joins function and status", () => {
    const text = describeService({
      service_key: "host-inspect",
      name: "Host Inspect",
      type: "agent",
      description: "",
      current_state: "running",
      state_summary: "alive",
      severity: "normal",
    });
    expect(text).toMatch(/主机巡检/);
    expect(text).toMatch(/alive/);
  });
});

describe("servicePathLabel", () => {
  it("labels http urls as probes and binaries as main process", () => {
    expect(servicePathLabel("https://yinger650.com/")).toBe("探测 URL");
    expect(servicePathLabel("/usr/sbin/nginx")).toBe("主进程");
  });
});
