import { afterEach, describe, expect, it, vi } from "vitest";
import { historyExport } from "./history-export";
import { APIError } from "@/shared/http";
const manifest = {
  type: "manifest",
  schema_version: 1,
  project_id: "project-one",
  workspace_id: "ws-one",
  high_watermark: 1,
};
const end = {
  type: "export_complete",
  high_watermark: 1,
  records: 1,
  sha256: "test",
};
function serve(lines: unknown[], status = 200, type = "application/x-ndjson") {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response(
          lines.map((line) => JSON.stringify(line)).join("\n") + "\n",
          { status, headers: { "Content-Type": type } },
        ),
      ),
  );
}
afterEach(() => vi.unstubAllGlobals());
describe("history export recovery", () => {
  it("accepts a complete scoped export without navigating", async () => {
    serve([manifest, { type: "record", content: "Résumé 日本語" }, end]);
    const blob = await historyExport("project-one", "ws-one");
    expect(await blob.text()).toContain("Résumé 日本語");
    expect(fetch).toHaveBeenCalledWith(
      "/ui-api/workspaces/ws-one/history/export",
      expect.objectContaining({ cache: "no-store", redirect: "error" }),
    );
  });
  it.each([
    [manifest, { type: "export_error", code: "HISTORY_UNAVAILABLE" }],
    [manifest, { type: "record" }],
    [manifest, { ...end, high_watermark: 2 }],
    [{ ...manifest, schema_version: 2 }, end],
  ])("rejects incomplete export envelopes: %j", async (...lines) => {
    serve(lines);
    await expect(historyExport("project-one", "ws-one")).rejects.toMatchObject({
      code: "HISTORY_EXPORT_INCOMPLETE",
    });
  });
  it("rejects a different project's private history", async () => {
    serve([{ ...manifest, project_id: "other" }, end]);
    await expect(historyExport("project-one", "ws-one")).rejects.toMatchObject({
      code: "PROJECT_MISMATCH",
    });
  });
  it("maps a failed export into the shared actionable error", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          Response.json({ code: "HISTORY_WORKSPACE_STOPPED" }, { status: 409 }),
        ),
    );
    await expect(historyExport("project-one", "ws-one")).rejects.toThrow(
      "Start the worker",
    );
  });
  it("rejects an HTML login/error page instead of downloading it as history", async () => {
    serve([], 200, "text/html");
    await expect(historyExport("project-one", "ws-one")).rejects.toBeInstanceOf(
      APIError,
    );
  });
  it("explains network failures", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new TypeError("Failed to fetch")),
    );
    await expect(historyExport("project-one", "ws-one")).rejects.toMatchObject({
      code: "NETWORK_UNAVAILABLE",
    });
  });
  it("does not offer a completed download after its view was closed", async () => {
    serve([manifest, end]);
    const reason = new DOMException("View closed", "AbortError");
    await expect(
      historyExport("project-one", "ws-one", AbortSignal.abort(reason)),
    ).rejects.toBe(reason);
  });
});
