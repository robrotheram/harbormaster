import { describe, expect, it } from "vitest";
import {
  checkedHermes,
  hermesRuntimeNotice,
  type HermesSnapshot,
} from "@/features/project-assistant/model";
import { APIError } from "@/shared/http";

const value: HermesSnapshot = {
  config: {
    project_id: "project-a",
    provider: "openai-codex",
    model: "gpt-6.1-sol",
    mode: "workspace",
    connection_profile_id: "",
    matrix: null,
    revision: 1,
    updated_at: null,
  },
  saved: true,
  runtime_available: false,
  runtime_status: "not_installed",
  matrix_available: false,
};
describe("Hermes project configuration", () => {
  it("clears the banner when a runtime becomes available", () => {
    expect(
      hermesRuntimeNotice({
        ...value,
        runtime_available: true,
        runtime_status: "available",
      }),
    ).toBeNull();
    expect(
      hermesRuntimeNotice({ ...value, runtime_status: "unavailable" })?.title,
    ).toBe("Could not check Hermes runtime");
    expect(hermesRuntimeNotice(value)?.title).toBe(
      "Hermes runtime unavailable",
    );
  });
  it("rejects another project's configuration before rendering it", () => {
    expect(() => checkedHermes(value, "project-b")).toThrow(APIError);
    expect(checkedHermes(value, "project-a").config.model).toBe("gpt-6.1-sol");
  });
  it("keeps a saved setup separate from runtime availability", () => {
    const result = checkedHermes(value, "project-a");
    expect(result.saved).toBe(true);
    expect(result.runtime_available).toBe(false);
    expect(result.matrix_available).toBe(false);
  });
});
