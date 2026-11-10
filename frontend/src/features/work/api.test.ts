import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import {
  workApi,
  workOptions,
  workKeys,
  evidenceURL,
  workActions,
} from "./api";
afterEach(() => vi.unstubAllGlobals());
const item = { id: "one", project_id: "A", state: "draft" };
function reply(value: unknown) {
  return vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(value), { status: 200 })),
  );
}
describe("work project boundary", () => {
  it("never uses project A's cached list while project B loads", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(workKeys.list("A"), [item]);
    reply({ items: [{ ...item, project_id: "B" }] });
    expect(client.getQueryData(workKeys.list("B"))).toBeUndefined();
    expect(await client.fetchQuery(workOptions("B"))).toEqual([
      { ...item, project_id: "B" },
    ]);
    expect(client.getQueryData(workKeys.list("A"))).toEqual([item]);
    client.clear();
  });
  it("rejects stale foreign project responses and mismatched details", async () => {
    reply({ items: [item] });
    await expect(workApi.list("B")).rejects.toThrow();
    reply(item);
    await expect(workApi.detail("B", "one")).rejects.toThrow();
    await expect(workApi.detail("A", "two")).rejects.toThrow();
    reply({ items: [{ work_item_id: "two" }] });
    await expect(workApi.attempts("A", "one")).rejects.toThrow();
  });
});
describe("work lifecycle contract", () => {
  it("scopes run, cancellation and both review decisions; retries retain the caller key", async () => {
    reply({});
    await workApi.run("A", "one", "delivery-key");
    await workApi.run("A", "one", "delivery-key");
    await workApi.review("A", "one", true);
    await workApi.review("A", "one", false);
    await workApi.cancel("A", "one");
    const calls = vi.mocked(fetch).mock.calls;
    expect(calls.map((c) => [c[0], JSON.parse(String(c[1]?.body))])).toEqual([
      [
        "/ui-api/work-items/one/run",
        { project_id: "A", idempotency_key: "delivery-key" },
      ],
      [
        "/ui-api/work-items/one/run",
        { project_id: "A", idempotency_key: "delivery-key" },
      ],
      ["/ui-api/work-items/one/review", { project_id: "A", accepted: true }],
      ["/ui-api/work-items/one/review", { project_id: "A", accepted: false }],
      ["/ui-api/work-items/one/cancel", { project_id: "A" }],
    ]);
  });
  it("offers lifecycle actions only for their applicable states", () => {
    expect(workActions("draft")).toEqual({
      run: true,
      cancel: false,
      review: false,
    });
    expect(workActions("running")).toEqual({
      run: false,
      cancel: true,
      review: false,
    });
    expect(workActions("awaiting_review")).toEqual({
      run: false,
      cancel: false,
      review: true,
    });
    expect(workActions("accepted")).toEqual({
      run: false,
      cancel: false,
      review: false,
    });
  });
  it("uses only returned safe evidence links", () => {
    expect(evidenceURL("/v1/artifacts/a?project_id=A")).toBe(
      "/v1/artifacts/a?project_id=A",
    );
    expect(evidenceURL("https://evidence.example/report")).toBe(
      "https://evidence.example/report",
    );
    for (const value of [
      "javascript:alert(1)",
      "file:///tmp/report",
      "//host/path",
      "/tmp/report",
      undefined,
    ])
      expect(evidenceURL(value)).toBeUndefined();
  });
});
