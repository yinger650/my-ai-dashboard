import { describe, expect, it } from "vitest";
import {
  BOARD_URL_PLACEHOLDER,
  GITHUB_CLIENT_DOWNLOAD,
  MACHINE_KEY_PLACEHOLDER,
  agentEnvSnippet,
  agentInstallPrompt,
  clientDownloadUrl,
  hostEnvSnippet,
  hostInstallPrompt,
  hostYamlSnippet,
} from "./install-prompt";

describe("agentInstallPrompt", () => {
  it("keeps {Machine Key} until a token is supplied", () => {
    const text = agentInstallPrompt("https://board.example", "");
    expect(text).toContain(MACHINE_KEY_PLACEHOLDER);
    expect(text).toContain("AGENTBOARD_TOKEN={Machine Key}");
    expect(text).toContain("https://board.example");
    expect(text).not.toContain("abp_m_secret");
  });

  it("substitutes the real machine token when present", () => {
    const text = agentInstallPrompt("https://board.example/abc12345", "abp_m_secret");
    expect(text).toContain("AGENTBOARD_TOKEN=abp_m_secret");
    expect(text).toContain("项目 Machine Token：abp_m_secret");
    expect(text).not.toContain(MACHINE_KEY_PLACEHOLDER);
  });
});

describe("host downloads and snippets", () => {
  it("prefers the board mirror, falls back to GitHub", () => {
    expect(clientDownloadUrl("https://board.example/", "board-client-linux-amd64")).toBe(
      "https://board.example/client-updates/board-client-linux-amd64",
    );
    expect(clientDownloadUrl("", "board-client-linux-amd64", true)).toBe(
      `${GITHUB_CLIENT_DOWNLOAD}/board-client-linux-amd64`,
    );
  });

  it("leaves {Machine Key} in host snippets until filled", () => {
    expect(hostEnvSnippet("")).toBe(`ABP_MACHINE_TOKEN=${MACHINE_KEY_PLACEHOLDER}`);
    expect(hostYamlSnippet("", "")).toContain(`url: "${BOARD_URL_PLACEHOLDER}"`);
    expect(hostInstallPrompt({
      boardUrl: "",
      machineKey: "",
      machineCode: "",
      downloadAmd64: "https://x/amd64",
      downloadArm64: "https://x/arm64",
    })).toContain(MACHINE_KEY_PLACEHOLDER);
  });

  it("writes agent .env with placeholders or values", () => {
    expect(agentEnvSnippet("https://b", "abp_m_x")).toContain("AGENTBOARD_TOKEN=abp_m_x");
  });
});
