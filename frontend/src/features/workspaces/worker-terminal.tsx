import { useEffect, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
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
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "'Geist Mono Variable', monospace",
      scrollback: 3000,
      theme: { background: "#101217", foreground: "#e3e6ed" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container);
    const resize = () => {
      if (disposed) return;
      const dimensions = fit.proposeDimensions();
      if (dimensions)
        term.resize(
          Math.max(2, Math.min(500, dimensions.cols)),
          Math.max(1, Math.min(200, dimensions.rows)),
        );
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(container);
    term.textarea?.focus({ preventScroll: true });
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
    const subscription = term.onData((data) => {
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
    });
    const exchange = async () => {
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
          term.reset();
          term.writeln("[Earlier terminal output was truncated]");
        }
        if (result.output) {
          const bytes = Uint8Array.from(atob(result.output), (c) =>
            c.charCodeAt(0),
          );
          await new Promise<void>((resolve) => term.write(bytes, resolve));
        }
        if (disposed) return;
        offset = result.offset;
        if (result.exited) {
          ended = true;
          setStatus("Shell exited");
          void close();
        } else {
          timer = setTimeout(exchange, input.length ? 0 : 100);
        }
      } catch (reason) {
        fail(reason);
      }
    };
    void request<{ id: string }>(base, { id, cols: term.cols, rows: term.rows })
      .then((result) => {
        created = true;
        if (result.id !== id)
          throw new Error("The terminal returned an unexpected session.");
        if (disposed) {
          void close();
          return;
        }
        setStatus("Connected");
        void exchange();
      })
      .catch(fail);
    const unload = () => {
      if (created) void close();
    };
    window.addEventListener("pagehide", unload);
    return () => {
      disposed = true;
      clearTimeout(timer);
      window.removeEventListener("pagehide", unload);
      observer.disconnect();
      subscription.dispose();
      term.dispose();
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
