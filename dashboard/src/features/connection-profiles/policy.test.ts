import { describe, it, expect } from "vitest";
import { parseDestinations, connectionChangeNeedsStop } from "./policy";
describe("connection policy editing", () => {
  it("normalizes the same pasted destinations for workspace and global profile editors", () => {
    expect(
      parseDestinations(
        " GITHUB.COM:443\r\n\ngithub.com:443\n Registry.NPMJS.org:443 ",
      ),
    ).toEqual(["github.com:443", "registry.npmjs.org:443"]);
  });
  it("requires restart for direct networking transitions but allows proxy updates", () => {
    const running = {
      state: "READY",
      connection_mode: "blocked",
    };
    expect(
      connectionChangeNeedsStop(running, { mode: "open", destinations: [] }),
    ).toBe(true);
    expect(
      connectionChangeNeedsStop(running, {
        mode: "allowlist",
        destinations: ["registry.npmjs.org:443"],
      }),
    ).toBe(false);
    expect(
      connectionChangeNeedsStop(
        { ...running, state: "STOPPED" },
        { mode: "open", destinations: [] },
      ),
    ).toBe(false);
  });
});
