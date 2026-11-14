import { useEffect, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { Button } from "@/components/ui/button";
import { ErrorNotice, Modal } from "@/shared/components";
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
  onClose,
}: {
  workspace: Workspace;
  onClose: () => void;
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
    term.focus();
    setError(undefined);
    setStatus("Connecting…");

    const close = () =>
      fetch(`/ui-api${base}/${id}/close`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Workbench-Request": "1",
        },
        body: "{}",
        keepalive: true,
        cache: "no-store",
        redirect: "error",
      }).catch(() => undefined);
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
  }, [workspace.id, generation, container]);

  return (
    <Modal
      title={`Terminal · ${workspace.name || workspace.id}`}
      onClose={onClose}
      wide
      closeOnEscape={false}
    >
      <p className="text-xs text-muted-foreground">
        Interactive shell in this worker’s workspace. Closing this window ends
        the shell. Ctrl+C interrupts a command.
      </p>
      {error != null && <ErrorNotice error={error} />}
      <div
        ref={setContainer}
        aria-label="Worker terminal"
        className="h-[min(60dvh,520px)] min-h-48 min-w-0 overflow-hidden rounded-md bg-[#101217] p-2"
      />
      <div className="flex items-center justify-between gap-3">
        <span role="status" className="text-xs text-muted-foreground">
          {status}
        </span>
        <div className="flex gap-2">
          {(status === "Disconnected" || status === "Shell exited") && (
            <Button
              variant="outline"
              onClick={() => setGeneration((value) => value + 1)}
            >
              New shell
            </Button>
          )}
          <Button variant="outline" onClick={onClose}>
            Close terminal
          </Button>
        </div>
      </div>
    </Modal>
  );
}
