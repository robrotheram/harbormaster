import { useEffect, useRef, useState } from "react";
import {
  ArrowDown,
  Copy,
  Download,
  Pause,
  Play,
  WrapText,
  MessageSquare,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorNotice, Modal, State } from "@/shared/components";

export type Application = {
  id: string;
  state: string;
  ready: boolean;
  logs?: string;
  application: {
    kind?: string;
    listener?: string;
    directory: string;
    command: string;
    port: number;
  };
};

export function ApplicationLogs({
  applications,
  initialID,
  error,
  onClose,
  onDebug,
}: {
  applications: Application[];
  initialID: string;
  error?: unknown;
  onClose: () => void;
  onDebug: (message: string) => void;
}) {
  const debugDraft = useRef("");
  const [id, setID] = useState(initialID);
  const [filter, setFilter] = useState("");
  const [frozen, setFrozen] = useState<string | null>(null);
  const [follow, setFollow] = useState(true);
  const [wrap, setWrap] = useState(true);
  const [copyStatus, setCopyStatus] = useState("");
  const viewport = useRef<HTMLPreElement>(null);
  const app = applications.find((a) => a.id === id);
  const content = frozen ?? app?.logs ?? "";
  const lines = content ? content.split(/\r?\n/) : [];
  const filtered = filter
    ? lines.filter((line) => line.toLowerCase().includes(filter.toLowerCase()))
    : lines;
  const shown = filtered.join("\n");
  useEffect(() => {
    if (follow && viewport.current)
      viewport.current.scrollTop = viewport.current.scrollHeight;
  }, [shown, follow]);
  useEffect(() => {
    setCopyStatus("");
  }, [shown]);
  function download() {
    const url = URL.createObjectURL(
      new Blob([content], { type: "text/plain;charset=utf-8" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = `${id}.log`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <Modal
      onCloseAutoFocus={(event) => {
        if (debugDraft.current) {
          event.preventDefault();
          onDebug(debugDraft.current);
        }
      }}
      title="Application logs"
      onClose={onClose}
      className="application-log-dialog"
    >
      <div className="application-log-context">
        <select
          aria-label="Log session"
          value={id}
          onChange={(e) => {
            setID(e.target.value);
            setFrozen(null);
            setFilter("");
            setFollow(true);
          }}
        >
          {applications.map((a) => (
            <option key={a.id} value={a.id}>
              {a.application.directory} · Port {a.application.port} · {a.state}{" "}
              · {a.id.slice(-6)}
            </option>
          ))}
        </select>
        {app && <State value={app.state} />}
      </div>
      {app && (
        <div className="application-log-command">
          <span>Command</span>
          <code>{app.application.command}</code>
          <span>Directory</span>
          <code>{app.application.directory}</code>
        </div>
      )}
      <div className="application-log-controls">
        <Input
          aria-label="Filter log lines"
          type="search"
          placeholder="Filter log lines…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <Button
          variant="outline"
          size="sm"
          onClick={() => setFrozen(frozen === null ? content : null)}
        >
          {frozen === null ? <Pause size={14} /> : <Play size={14} />}{" "}
          {frozen === null ? "Pause" : "Resume"}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-pressed={wrap}
          onClick={() => setWrap(!wrap)}
        >
          <WrapText size={14} />
          Wrap
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={!shown}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(shown);
              setCopyStatus("Copied visible logs.");
            } catch {
              setCopyStatus(
                "Copy unavailable. Select the log text or download it.",
              );
            }
          }}
        >
          <Copy size={14} />
          Copy
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={!content}
          onClick={download}
        >
          <Download size={14} />
          Download
        </Button>
      </div>
      {!!error && <ErrorNotice error={error} />}
      {!app && !error && (
        <p role="status">This session is no longer available.</p>
      )}
      <pre
        ref={viewport}
        tabIndex={0}
        aria-label="Application log output"
        className={`application-log-output ${wrap ? "is-wrapped" : ""}`}
        onScroll={() => {
          const el = viewport.current!;
          if (el.scrollHeight - el.scrollTop - el.clientHeight > 60)
            setFollow(false);
        }}
      >
        {shown ||
          (content
            ? "No lines match this filter."
            : app?.application.kind === "forward"
              ? "This app was started outside the preview launcher. Look in the conversation’s tool activity for its process output."
              : app?.state === "active"
                ? "No application output yet. New stdout and stderr messages will appear here."
                : "No output is available for this session.")}
      </pre>
      <div className="application-log-footer">
        <div>
          <span>
            {frozen !== null ? "Paused" : "Updates every 3 seconds"} ·{" "}
            {filtered.length} {filter ? "matching" : "log"} lines
          </span>
          <small>
            Latest 64 KB of application output. Logs are cleared when the
            workspace restarts.
          </small>
          <span role="status">{copyStatus}</span>
        </div>
        <div className="application-log-actions">
          <Button
            variant="ghost"
            size="sm"
            aria-pressed={follow}
            onClick={() => setFollow(!follow)}
          >
            <ArrowDown size={14} />
            Follow latest
          </Button>
          <Button
            size="sm"
            disabled={!app || !shown}
            onClick={() => {
              debugDraft.current = `Help debug this application.\nDirectory: ${app!.application.directory}\nCommand: ${app!.application.command}\nPort: ${app!.application.port}\nSession state: ${app!.state}\n\nRecent${filter ? " filtered" : ""} application logs${shown.length > 6000 ? " (last 6,000 characters)" : ""}:\n${shown.slice(-6000)}\n\nIdentify the cause and propose a fix.\n`;
              onClose();
            }}
          >
            <MessageSquare size={14} />
            Ask agent to debug
          </Button>
        </div>
      </div>
    </Modal>
  );
}
