import { APIError } from "@/shared/http";
import type { Pairing } from "@/shared/types";
export function consoleURL(id: string, ticket: string, port: string): string {
  if (
    !/^ws_[a-f0-9]{32}$/.test(id) ||
    !ticket ||
    (port && !/^\d{1,5}$/.test(port))
  )
    throw new APIError("INVALID_REQUEST", 400);
  return (
    "http://" +
    id.replace("ws_", "ws-") +
    ".localhost" +
    (port ? ":" + port : "") +
    "/connect#ticket=" +
    encodeURIComponent(ticket)
  );
}

export function safePairing(value: Pairing, workspaceID?: string): Pairing {
  const url = new URL(value.pairing_url);
  if (
    url.protocol !== "http:" ||
    url.username ||
    url.password ||
    url.search ||
    !/^desktop-ws-[a-f0-9]{32}\.localhost$/.test(url.hostname) ||
    (workspaceID !== undefined &&
      url.hostname !==
        "desktop-" + workspaceID.replace("ws_", "ws-") + ".localhost") ||
    url.pathname !== "/pair" ||
    !new URLSearchParams(url.hash.slice(1)).get("token")
  )
    throw new APIError("PAIRING_UNAVAILABLE", 503);
  return value;
}
