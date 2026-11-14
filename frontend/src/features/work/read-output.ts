import { APIError } from "@/shared/http";
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
