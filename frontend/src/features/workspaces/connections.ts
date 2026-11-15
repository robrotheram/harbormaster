import { APIError } from "@/shared/http";
import type { Pairing } from "@/shared/types";
export function consoleURL(
  id: string,
  ticket: string,
  port: string,
  publicOrigin = "",
): string {
  if (
    !/^ws_[a-z0-9][a-z0-9-]{0,60}$/.test(id) ||
    !ticket ||
    (port && !/^\d{1,5}$/.test(port))
  )
    throw new APIError("INVALID_REQUEST", 400);
  const hosted = publicOrigin ? new URL(publicOrigin) : undefined;
  if (hosted?.protocol === "https:")
    return `https://${id.replace("ws_", "ws-")}.${hosted.host}/connect#ticket=${encodeURIComponent(ticket)}`;
  return (
    "http://" +
    id.replace("ws_", "ws-") +
    ".localhost" +
    (port ? ":" + port : "") +
    "/connect#ticket=" +
    encodeURIComponent(ticket)
  );
}

export function safePairing(
  value: Pairing,
  workspaceID?: string,
  publicOrigin = "",
): Pairing {
  const url = new URL(value.pairing_url);
  const hosted = publicOrigin ? new URL(publicOrigin) : undefined;
  const hostedHost =
    hosted?.protocol === "https:" && workspaceID
      ? `desktop-${workspaceID.replace("ws_", "ws-")}.${hosted.host}`
      : undefined;
  if (
    (hostedHost
      ? url.protocol !== "https:" || url.host !== hostedHost
      : url.protocol !== "http:") ||
    url.username ||
    url.password ||
    url.search ||
    (!hostedHost &&
      !/^desktop-ws-[a-z0-9][a-z0-9-]{0,60}\.localhost$/.test(url.hostname)) ||
    (!hostedHost &&
      workspaceID !== undefined &&
      url.hostname !==
        "desktop-" + workspaceID.replace("ws_", "ws-") + ".localhost") ||
    url.pathname !== "/pair" ||
    !new URLSearchParams(url.hash.slice(1)).get("token")
  )
    throw new APIError("PAIRING_UNAVAILABLE", 503);
  return value;
}
