import { useRef, useState, type FormEvent, type ReactNode } from "react";
import { AlertCircle, Inbox, Clock3 } from "lucide-react";
import { date, label, stateTone } from "@/shared/format";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { APIError } from "@/shared/http";
import { cn } from "@/lib/utils";

export function State({ value }: { value: string }) {
  const tone = stateTone(value);
  return (
    <Badge
      variant="outline"
      className={cn(
        "gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium",
        tone === "good"
          ? "border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-900 dark:bg-teal-950 dark:text-teal-300"
          : tone === "active"
            ? "border-indigo-200 bg-indigo-50 text-indigo-700 dark:border-indigo-900 dark:bg-indigo-950 dark:text-indigo-300"
            : tone === "warn"
              ? "border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300"
              : "border-border bg-muted text-muted-foreground",
      )}
    >
      <span
        className={cn(
          "size-1.5 rounded-full bg-current",
          value === "STARTING" && "motion-safe:animate-pulse",
        )}
      />
      {label(value)}
    </Badge>
  );
}
export function Empty({
  title,
  children,
  action,
}: {
  title: string;
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center rounded-xl border border-dashed bg-background px-6 py-12 text-center">
      <span className="mb-5 flex size-12 items-center justify-center rounded-xl border bg-muted/50 text-muted-foreground">
        <Inbox className="size-5" />
      </span>
      <h3 className="text-base font-semibold">{title}</h3>
      <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
        {children}
      </p>
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}
export function ErrorNotice({
  error,
  message,
  onRetry,
  retrying = false,
}: {
  error: unknown;
  message?: string;
  onRetry?: () => void;
  retrying?: boolean;
}) {
  return (
    <div
      className="mb-4 flex items-start gap-2.5 rounded-lg border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive"
      role="alert"
    >
      <AlertCircle className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 flex-1 break-words">
        <p>
          {message ??
            (error instanceof Error
              ? error.message
              : "This operation is unavailable. Check its current state before trying again.")}
        </p>
        {error instanceof APIError && (
          <details className="mt-2 text-xs text-muted-foreground">
            <summary className="cursor-pointer">Support details</summary>
            {message && <p className="mt-2">{error.message}</p>}
            <p className="mt-2 font-mono">
              Code: {error.code}
              {error.status > 0 ? ` · HTTP ${error.status}` : ""}
            </p>
          </details>
        )}
        {onRetry && (
          <Button
            type="button"
            className="mt-3"
            variant="outline"
            disabled={retrying}
            onClick={onRetry}
          >
            {retrying ? "Retrying…" : "Try again"}
          </Button>
        )}
      </div>
    </div>
  );
}
export function Observation({
  at,
  fetching,
  error,
}: {
  at: number;
  fetching: boolean;
  error: boolean;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-xs text-muted-foreground",
        error && "text-amber-700 dark:text-amber-400",
      )}
    >
      <Clock3 className="size-3" />
      {error
        ? "Last successful update"
        : fetching
          ? "Refreshing"
          : "Updated"}{" "}
      {at ? date(new Date(at).toISOString()) : "—"}
    </span>
  );
}
export function Modal({
  title,
  onClose,
  children,
  wide = false,
  className,
  closeOnEscape = true,
  closeDisabled = false,
  onCloseAutoFocus,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
  className?: string;
  closeOnEscape?: boolean;
  closeDisabled?: boolean;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const [returnFocus] = useState(() => {
    const active = document.activeElement as HTMLElement | null;
    const menu = active?.closest('[role="menu"]');
    const trigger = menu?.getAttribute("aria-labelledby");
    return trigger ? document.getElementById(trigger) : active;
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !closeDisabled) onClose();
      }}
    >
      <DialogContent
        closeButtonDisabled={closeDisabled}
        onEscapeKeyDown={(event) => {
          if (!closeOnEscape || closeDisabled) event.preventDefault();
        }}
        onCloseAutoFocus={(event) => {
          onCloseAutoFocus?.(event);
          if (event.defaultPrevented) return;
          if (returnFocus?.isConnected) {
            event.preventDefault();
            requestAnimationFrame(() => returnFocus.focus());
          }
        }}
        aria-describedby={undefined}
        className={cn(
          "max-h-[90dvh] overflow-y-auto p-6",
          wide ? "sm:max-w-3xl" : "sm:max-w-[540px]",
          className,
        )}
      >
        <DialogHeader className="mb-1 pr-7 text-left">
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}
export function Form({
  onSubmit,
  children,
}: {
  onSubmit: () => void;
  children: ReactNode;
}) {
  return (
    <form
      className="space-y-5"
      onSubmit={(event: FormEvent) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      {children}
    </form>
  );
}
export function useRequestKey() {
  const last = useRef({ body: "", key: "" });
  return (payload: unknown) => {
    const body = JSON.stringify(payload);
    if (body !== last.current.body || !last.current.key)
      last.current = { body, key: crypto.randomUUID() };
    return last.current.key;
  };
}
export function WorkspaceName({ name, id }: { name?: string; id: string }) {
  return (
    <span className="flex min-w-0 flex-col gap-1">
      <strong className="truncate text-sm font-semibold">
        {name || "Untitled workspace"}
      </strong>
      <span
        className="max-w-56 truncate font-mono text-[11px] text-muted-foreground"
        title={id}
      >
        {id}
      </span>
    </span>
  );
}
