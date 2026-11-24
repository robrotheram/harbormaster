import { useEffect, useState } from "react";
import { FitAddon, Ghostty, Terminal } from "ghostty-web";
import ghosttyWasm from "ghostty-web/ghostty-vt.wasm?url";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import type { Workspace } from "@/shared/types";

type Exchange = {
  output: string;
  offset: number;
  exited: boolean;
  truncated: boolean;
};

let ghostty: Promise<Ghostty> | undefined;

function loadGhostty() {
  // Use Vite's same-origin asset URL under the dashboard's CSP.
  return (ghostty ??= Ghostty.load(ghosttyWasm).catch((error) => {
    ghostty = undefined;
    throw error;
  }));
}

export function WorkerTerminal({
  workspace,
  onSessionClosed,
}: {
  workspace: Workspace;
  onSessionClosed: () => void;
}) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null);
  const [status, setStatus] = useState("Connecting…");
  const [error, setError] = useState<unknown>();
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    if (!container) return;
    const id = "terminal_" + crypto.randomUUID().replaceAll("-", "");
    const base = `/workspaces/${encodeURIComponent(workspace.id)}/terminals`;
    let disposed = false;
    let ended = false;
    let created = false;
    let timer: ReturnType<typeof setTimeout>;
    let offset = 0;
    let input: string[] = [];
    let disposeTerminal: (() => void) | undefined;
    setError(undefined);
    setStatus("Connecting…");

    let closing: Promise<Response | undefined> | undefined;
    const close = () =>
      (closing ??= fetch(`/ui-api${base}/${id}/close`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Workbench-Request": "1",
        },
        body: "{}",
        keepalive: true,
        cache: "no-store",
        redirect: "error",
      })
        .catch(() => undefined)
        .finally(onSessionClosed));
    const fail = (reason: unknown) => {
      ended = true;
      input = [];
      if (!disposed) {
        setError(reason);
        setStatus("Disconnected");
      }
      if (created) void close();
    };
    const queueInput = (data: string) => {
      if (!created || ended || disposed) return;
      if (input.length + data.length > 65536) {
        fail(
          new Error(
            "Terminal input is full. Reopen the shell before sending more input.",
          ),
        );
        return;
      }
      input.push(...Array.from(data));
    };
    const exchange = async (term: Terminal) => {
      if (disposed || ended) return;
      try {
        const result = await request<Exchange>(`${base}/${id}/exchange`, {
          input: input.splice(0, 1024).join(""),
          offset,
          cols: term.cols,
          rows: term.rows,
        });
        if (disposed) return;
        if (result.truncated) {
          // Cancel any partial escape sequence and reset in place so selection
          // keeps using the live Ghostty buffer.
          term.clearSelection();
          term.writeln("\x18\x1bc[Earlier terminal output was truncated]");
        }
        if (result.output) {
          const bytes = Uint8Array.from(atob(result.output), (c) =>
            c.charCodeAt(0),
          );
          term.write(bytes);
        }
        offset = result.offset;
        if (result.exited) {
          ended = true;
          setStatus("Shell exited");
          void close();
        } else {
          timer = setTimeout(() => void exchange(term), input.length ? 0 : 100);
        }
      } catch (reason) {
        fail(reason);
      }
    };
    const connect = async () => {
      const ghostty = await loadGhostty();
      if (disposed) return;
      const surface = document.createElement("div");
      // Ghostty draws its cursor on the canvas and uses this element for text input.
      surface.className = "relative size-full caret-transparent";
      const term = new Terminal({
        ghostty,
        cursorBlink: true,
        fontSize: 13,
        fontFamily: "'Geist Mono Variable', monospace",
        scrollback: 3000,
        theme: { background: "#101217", foreground: "#e3e6ed" },
      });
      const fit = new FitAddon();
      const resize = () => {
        if (disposed) return;
        const dimensions = fit.proposeDimensions();
        if (dimensions)
          term.resize(
            Math.max(2, Math.min(500, dimensions.cols)),
            Math.max(1, Math.min(200, dimensions.rows)),
          );
      };
      const observer = new ResizeObserver(resize);
      const subscription = term.onData(queueInput);
      disposeTerminal = () => {
        observer.disconnect();
        subscription.dispose();
        term.dispose();
        surface.remove();
      };
      container.append(surface);
      term.loadAddon(fit);
      term.open(surface);
      // The canvas handles focus on click. Only the terminal surface needs a tab stop.
      if (term.textarea) term.textarea.tabIndex = -1;
      resize();
      observer.observe(surface);

      const result = await request<{ id: string }>(base, {
        id,
        cols: term.cols,
        rows: term.rows,
      });
      created = true;
      if (result.id !== id)
        throw new Error("The terminal returned an unexpected session.");
      if (disposed) {
        void close();
        return;
      }
      setStatus("Connected");
      void exchange(term);
    };
    void connect().catch(fail);
    const unload = () => {
      if (created) void close();
    };
    window.addEventListener("pagehide", unload);
    return () => {
      disposed = true;
      clearTimeout(timer);
      window.removeEventListener("pagehide", unload);
      disposeTerminal?.();
      if (created) void close();
    };
  }, [workspace.id, generation, container, onSessionClosed]);

  return (
    <section
      aria-label="Worker terminal"
      className="min-w-0 overflow-hidden rounded-md border bg-card"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <code className="text-xs font-medium">/data/files</code>
          <span
            role="status"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
          >
            <span
              aria-hidden="true"
              className={`size-1.5 rounded-full ${status === "Connected" ? "bg-teal-600 dark:bg-teal-400" : "bg-current"}`}
            />
            {status}
          </span>
        </div>
        <div className="flex gap-2">
          {(status === "Disconnected" || status === "Shell exited") && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setGeneration((value) => value + 1)}
            >
              New shell
            </Button>
          )}
        </div>
      </div>
      {error != null && <ErrorNotice error={error} />}
      <div
        ref={setContainer}
        className="h-[max(20rem,calc(100dvh-21rem))] min-w-0 overflow-hidden bg-[#101217] p-2 sm:h-[max(22rem,calc(100dvh-18rem))] sm:p-3"
      />
    </section>
  );
}
