import { afterEach, expect, it, vi } from "vitest";
import { APIError, request } from "./http";
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
it("translates network failure without leaking browser details", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockRejectedValue(new TypeError("Failed to fetch secret-address")),
  );
  await expect(request("/projects")).rejects.toMatchObject({
    code: "NETWORK_UNAVAILABLE",
    status: 0,
  });
  await expect(request("/projects")).rejects.toThrow("Check your connection");
});
it("preserves caller cancellation", async () => {
  const controller = new AbortController();
  controller.abort();
  const error = new DOMException("Cancelled", "AbortError");
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(error));
  await expect(request("/projects", undefined, controller.signal)).rejects.toBe(
    error,
  );
});
it("explains uncertain timeout without encouraging duplicate changes", async () => {
  const signal = AbortSignal.abort(new DOMException("Timeout", "TimeoutError"));
  vi.spyOn(AbortSignal, "timeout").mockReturnValue(signal);
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(signal.reason));
  await expect(request("/projects", { name: "Example" })).rejects.toMatchObject(
    { code: "REQUEST_TIMEOUT" },
  );
});
it.each([401, 403, 429, 503])(
  "handles non-JSON HTTP %s responses",
  async (status) => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response("<html>Proxy error</html>", { status }),
        ),
    );
    await expect(request("/projects")).rejects.toThrow(
      new APIError("UNAVAILABLE", status).message,
    );
  },
);
