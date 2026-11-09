import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { APIError, request, requireProject } from "@/shared/http";
import { consoleURL, safePairing } from "@/features/workspaces/connections";
import { keys } from "@/shared/query-keys";
import { workspacesApi } from "@/features/workspaces/api";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("project data boundaries", () => {
  it("never substitutes cached data when the active project changes", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(keys.workspaces("project-a"), [
      { id: "a-private", project_id: "project-a" },
    ]);
    const observer = new QueryObserver(client, {
      queryKey: keys.workspaces("project-a"),
      queryFn: async () => [{ id: "a-private", project_id: "project-a" }],
      staleTime: Infinity,
    });
    const unsubscribe = observer.subscribe(() => {});
    expect(observer.getCurrentResult().data?.[0].id).toBe("a-private");
    observer.setOptions({
      queryKey: keys.workspaces("project-b"),
      queryFn: async () => [{ id: "b-private", project_id: "project-b" }],
      staleTime: Infinity,
    });
    expect(observer.getCurrentResult().data).toBeUndefined();
    await observer.refetch();
    expect(observer.getCurrentResult().data?.[0].id).toBe("b-private");
    expect(
      client.getQueryData<{ id: string }[]>(keys.workspaces("project-a"))?.[0]
        .id,
    ).toBe("a-private");
    unsubscribe();
    client.clear();
  });

  it("rejects a backend response containing another project instead of rendering it", () => {
    const foreign = [
      { project_id: "project-a", secret: "sensitive-project-data" },
    ];
    expect(() => requireProject(foreign, "project-b")).toThrow(APIError);
    try {
      requireProject(foreign, "project-b");
    } catch (error) {
      expect((error as Error).message).not.toContain("sensitive-project-data");
    }
    expect(requireProject([], "project-b")).toEqual([]);
  });

  it("binds scoped requests to the requested project and rejects mismatched results", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [{ id: "foreign", project_id: "project-b" }],
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetch);
    await expect(workspacesApi.list("project-a")).rejects.toThrow(APIError);
    expect(fetch.mock.calls[0][0]).toBe(
      "/ui-api/workspaces?project_id=project-a",
    );
    expect(keys.homes("a")).not.toEqual(keys.homes("b"));
    expect(keys.runs("a")).not.toEqual(keys.runs("b"));
    expect(keys.schedules("a")).not.toEqual(keys.schedules("b"));
  });
});

describe("transport safety", () => {
  it("sends mutations as protected local JSON without management credentials", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(new Response('{"id":"created"}', { status: 201 }));
    vi.stubGlobal("fetch", fetch);
    await request("/projects", {
      name: "New project",
      idempotency_key: "request-one",
    });
    const options = fetch.mock.calls[0][1];
    expect(options.headers).toEqual({
      "Content-Type": "application/json",
      "X-Workbench-Request": "1",
    });
    expect(options.method).toBe("POST");
    expect(options.redirect).toBe("error");
    expect(options.cache).toBe("no-store");
  });

  it("never reflects arbitrary upstream error details", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            '{"code":"UPSTREAM_INTERNAL","message":"sensitive-upstream-body"}',
            { status: 503 },
          ),
        ),
    );
    await expect(request("/projects")).rejects.not.toThrow(
      "sensitive-upstream-body",
    );
  });

  it("keeps browser connection tickets in fragments and only accepts managed workspace hosts", () => {
    const value = consoleURL(
      "ws_0123456789abcdef0123456789abcdef",
      "one-use:token",
      "8080",
    );
    const url = new URL(value);
    expect(url.search).toBe("");
    expect(new URLSearchParams(url.hash.slice(1)).get("ticket")).toBe(
      "one-use:token",
    );
    expect(() => consoleURL("https://evil.example", "ticket", "8080")).toThrow(
      APIError,
    );
    expect(() =>
      consoleURL("ws_0123456789abcdef0123456789abcdef", "ticket", "8080/path"),
    ).toThrow(APIError);
  });

  it("rejects desktop pairing URLs outside the desktop origin or with embedded URL credentials", () => {
    const valid = {
      pairing_url:
        "http://desktop-ws-0123456789abcdef0123456789abcdef.localhost:8080/pair#token=one-use",
      expires_at: "2026-11-01T12:00:00Z",
    };
    expect(safePairing(valid)).toEqual(valid);
    expect(() =>
      safePairing(valid, "ws_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
    ).toThrow(APIError);
    for (const url of [
      "https://evil.example/pair#token=x",
      valid.pairing_url.replace("http://", "http://user:password@"),
      valid.pairing_url.replace("/pair#", "/pair?token=leak#"),
    ]) {
      expect(() => safePairing({ ...valid, pairing_url: url })).toThrow(
        APIError,
      );
    }
  });
});
