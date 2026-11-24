import { APIError } from "@/shared/http";

// Check the export envelope before offering a download. A stream can return 200
// and still end in export_error if the worker stops partway through the read.
export async function historyExport(
  project: string,
  workspace: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const timeout = AbortSignal.timeout(45000);
  try {
    const response = await fetch(
      `/ui-api/workspaces/${encodeURIComponent(workspace)}/history/export`,
      {
        cache: "no-store",
        redirect: "error",
        signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
      },
    );
    if (!response.ok) {
      const error = await response.json().catch(() => undefined);
      throw new APIError(error?.code || "UNAVAILABLE", response.status);
    }
    if (
      !response.headers.get("content-type")?.startsWith("application/x-ndjson")
    )
      throw new APIError("HISTORY_EXPORT_INCOMPLETE", 0);
    const blob = await response.blob();
    const head = await blob.slice(0, 65536).text();
    const tail = await blob.slice(Math.max(0, blob.size - 65536)).text();
    let manifest, end;
    try {
      manifest = JSON.parse(head.split("\n")[0]);
      end = JSON.parse(tail.trimEnd().split("\n").at(-1)!);
    } catch {
      throw new APIError("HISTORY_EXPORT_INCOMPLETE", 0);
    }
    if (manifest.project_id !== project || manifest.workspace_id !== workspace)
      throw new APIError("PROJECT_MISMATCH", 403);
    if (
      manifest.type !== "manifest" ||
      manifest.schema_version !== 1 ||
      !Number.isInteger(manifest.high_watermark) ||
      manifest.high_watermark < 0 ||
      end.type !== "export_complete" ||
      end.high_watermark !== manifest.high_watermark
    )
      throw new APIError("HISTORY_EXPORT_INCOMPLETE", 0);
    signal?.throwIfAborted();
    return blob;
  } catch (error) {
    if (signal?.aborted || error instanceof APIError) throw error;
    throw new APIError(
      timeout.aborted ? "REQUEST_TIMEOUT" : "NETWORK_UNAVAILABLE",
      0,
    );
  }
}
