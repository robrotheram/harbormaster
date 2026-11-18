import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, CircleAlert, Loader2, Play, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorNotice, Modal } from "@/shared/components";
import type { Workspace } from "@/shared/types";
import { workspacesApi } from "./api";

export function StartWorker({
  workspace,
  onClose,
  onStarted,
  onSettings,
}: {
  workspace: Workspace;
  onClose: () => void;
  onStarted: () => void;
  onSettings: () => void;
}) {
  const query = useQuery({
    queryKey: ["start-readiness", workspace.project_id, workspace.id],
    queryFn: ({ signal }) => workspacesApi.readiness(workspace, signal),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<unknown>();
  const [key] = useState(() => crypto.randomUUID());
  const ready = !query.isFetching && !query.isError && query.data?.can_start;
  const checks = query.data?.checks || [];
  const settings = checks.some((c) =>
    ["INVALID_CONNECTION_PROFILE", "NETWORK_POLICY_UNAVAILABLE"].includes(
      c.code || "",
    ),
  );
  const capacity = checks.some((c) => c.code === "QUOTA_EXCEEDED");
  const hermes = checks.some((c) => c.code === "HERMES_SETUP_REQUIRED");
  async function start() {
    if (!ready || starting) return;
    setStarting(true);
    setError(undefined);
    try {
      await workspacesApi.start(workspace.id, key);
      onStarted();
    } catch (err) {
      setError(err);
      await query.refetch();
    } finally {
      setStarting(false);
    }
  }
  return (
    <Modal
      title="Start worker"
      closeDisabled={starting}
      onClose={() => {
        if (!starting) onClose();
      }}
    >
      <div className="min-w-0">
        <p
          className="truncate font-medium"
          title={workspace.name || workspace.id}
        >
          {workspace.name || "Untitled worker"}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          Check startup requirements before allocating compute.
        </p>
      </div>
      <div
        role="status"
        className="flex items-center gap-2 border-y py-3 text-sm font-medium"
      >
        {query.isFetching || starting ? (
          <Loader2 className="size-4 loading-spinner" />
        ) : ready ? (
          <Check className="size-4 text-teal-600 dark:text-teal-400" />
        ) : (
          <CircleAlert className="size-4 text-amber-600 dark:text-amber-400" />
        )}
        {starting
          ? "Starting worker…"
          : query.isFetching
            ? "Checking startup…"
            : query.isError
              ? "Startup check unavailable"
              : ready
                ? "Startup checks passed"
                : "Action needed before starting"}
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      {error != null && <ErrorNotice error={error} />}
      {!query.isFetching && !query.isError && (
        <ul className="space-y-4" aria-label="Startup checks">
          {checks.map((check) => (
            <li key={check.id} className="flex gap-2.5">
              {check.status === "passed" ? (
                <Check
                  className="mt-0.5 size-3.5 shrink-0 text-muted-foreground"
                  aria-label="Passed"
                />
              ) : (
                <CircleAlert
                  className="mt-0.5 size-3.5 shrink-0 text-amber-600 dark:text-amber-400"
                  aria-label={
                    check.status === "blocked" ? "Blocked" : "Unverified"
                  }
                />
              )}
              <div className="min-w-0">
                <p className="text-xs font-medium">{check.label}</p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {check.message}
                </p>
              </div>
            </li>
          ))}
        </ul>
      )}
      {!query.isFetching && !ready && (
        <div className="flex flex-wrap gap-2">
          {settings && (
            <Button
              variant="outline"
              size="sm"
              disabled={starting}
              onClick={onSettings}
            >
              Worker settings
            </Button>
          )}
          {hermes && (
            <Button variant="outline" size="sm" asChild>
              <Link
                to="/projects/$projectId/$section"
                params={{
                  projectId: workspace.project_id,
                  section: "settings",
                }}
                search={{ tab: "hermes" }}
              >
                Hermes setup
              </Link>
            </Button>
          )}
          <Button variant="outline" size="sm" asChild>
            <Link to={capacity ? "/infrastructure" : "/platform-services"}>
              {capacity ? "Infrastructure" : "Platform services"}
            </Link>
          </Button>
        </div>
      )}
      <p className="text-xs leading-5 text-muted-foreground">
        Checking does not start the worker or reserve capacity. Starting checks
        capacity again; runtime startup can still fail.
      </p>
      <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
        <Button variant="ghost" disabled={starting} onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="outline"
          disabled={query.isFetching || starting}
          onClick={() => {
            setError(undefined);
            void query.refetch();
          }}
        >
          <RefreshCw />
          Check again
        </Button>
        <Button disabled={!ready || starting} onClick={start}>
          <Play />
          {starting ? "Starting…" : "Start worker"}
        </Button>
      </div>
    </Modal>
  );
}
