import { APIError, request } from "@/shared/http";
export async function readOutput(url: string, signal: AbortSignal) {
  const response = await fetch(url, {
    signal,
    cache: "no-store",
    redirect: "error",
  });
  if (!response.ok) throw new APIError("ARTIFACT_UNAVAILABLE", response.status);
  const reader = response.body?.getReader();
  if (!reader) throw new APIError("ARTIFACT_UNAVAILABLE", 503);
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > 262144)
        throw new Error(
          "This output is too large to preview. Download it to inspect the full file.",
        );
      chunks.push(value);
    }
  } finally {
    await reader.cancel();
  }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const c of chunks) {
    bytes.set(c, offset);
    offset += c.length;
  }
  return new TextDecoder().decode(bytes);
}

export async function readWorkspaceData(
  project: string,
  attempt: { work_item_id: string; id: string },
  path: string,
  signal: AbortSignal,
) {
  const file = await request<{ path: string; content: string }>(
    `/work-items/${attempt.work_item_id}/data`,
    { project_id: project, attempt_id: attempt.id, path },
    signal,
  );
  if (file.path !== path)
    throw new Error("The worker returned a different file.");
  const content = atob(file.content || "");
  if (content.length > 262144)
    throw new Error("Use data smaller than 256 KiB.");
  return new TextDecoder().decode(
    Uint8Array.from(content, (c) => c.charCodeAt(0)),
  );
}
