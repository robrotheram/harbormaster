import type { ReactNode } from "react";
import { Accordion } from "radix-ui";
import { ChevronDown, Package, Sparkles, type LucideIcon } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/shared/components";
import { APIError } from "@/shared/http";
import type { Workspace } from "@/shared/types";
import {
  changeWorkerSetup,
  readWorkerSetup,
  type SetupInventory,
  type SetupOperation,
} from "./setup-api";

import { SetupSkills } from "./worker-setup-skills";
import { SetupTools } from "./worker-setup-tools";

export function WorkerSetup({
  workspace,
  onSettings,
}: {
  workspace: Workspace;
  onSettings: () => void;
}) {
  const id = workspace.id;
  const client = useQueryClient();
  const key = ["worker-setup", id] as const;

  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => readWorkerSetup(id, signal),
    enabled: workspace.ready,
    refetchInterval: (query) =>
      query.state.data?.job?.state === "preparing" ? 1000 : 6000,
    retry: false,
  });
  const action = useMutation({
    mutationFn: (operation: SetupOperation) => changeWorkerSetup(id, operation),
    onSuccess: (job) => {
      client.setQueryData<SetupInventory>(key, (previous) =>
        previous ? { ...previous, job } : previous,
      );
      void client.invalidateQueries({ queryKey: key });
    },
    onError: () => {
      void client.invalidateQueries({ queryKey: key });
    },
  });

  function change(operation: SetupOperation) {
    action.mutate(operation);
  }

  const inventory = query.data;
  const job = inventory?.job;
  const pending = job?.state === "preparing" || action.isPending;
  const reviewing = job?.state === "review";
  const locked = pending || reviewing;
  const isBusy = workspace.state === "BUSY" && !pending;
  const errorCode =
    action.error instanceof APIError ? action.error.code : job?.code;
  const hasNetworkProblem =
    errorCode === "SETUP_DOWNLOAD_FAILED" ||
    errorCode === "SETUP_NETWORK_BLOCKED";
  const retry = job?.state === "failed" ? job.request : undefined;
  const ready = workspace.ready;

  return (
    <section aria-label="Worker environment" className="min-w-0 space-y-4">
      {!ready && (
        <div className="rounded-md border p-4 text-sm">
          <p>Start this worker to inspect and manage its environment.</p>
        </div>
      )}
      {ready && query.isPending && (
        <p role="status" className="text-sm text-muted-foreground">
          Checking the environment…
        </p>
      )}
      {ready && query.isError && (
        <ErrorNotice
          error={query.error}
          message={
            query.error instanceof APIError &&
            query.error.code === "PROJECT_ADMIN_REQUIRED"
              ? "Ask a project administrator to manage this worker’s environment."
              : undefined
          }
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {ready && inventory && (
        <>
          {isBusy && (
            <p className="text-sm text-muted-foreground">
              Finish active work and close terminals before changing the
              environment.
            </p>
          )}

          {pending && (
            <p
              role="status"
              className="rounded-md border bg-muted/30 p-3 text-sm"
            >
              {action.variables?.operation === "apply"
                ? "Applying changes…"
                : "Preparing changes…"}
            </p>
          )}
          {reviewing && job.review && (
            <section
              aria-labelledby="setup-review-heading"
              className="space-y-3 rounded-md border border-primary/40 bg-card p-4"
            >
              <h2 id="setup-review-heading" className="font-semibold">
                Review environment changes
              </h2>
              <ul className="list-disc space-y-1 break-words pl-5 text-sm">
                {job.review.changes.map((line) => (
                  <li key={line}>{line}</li>
                ))}
              </ul>
              {job.review.preview && (
                <details>
                  <summary className="w-fit cursor-pointer text-sm font-medium">
                    Preview skill instructions
                  </summary>
                  <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-background p-3 text-xs">
                    {job.review.preview}
                  </pre>
                </details>
              )}
              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  variant="outline"
                  disabled={pending}
                  onClick={() =>
                    change({ operation: "discard", plan_id: job.id })
                  }
                >
                  Discard
                </Button>
                <Button
                  disabled={pending || isBusy}
                  onClick={() =>
                    change({ operation: "apply", plan_id: job.id })
                  }
                >
                  Apply changes
                </Button>
              </div>
            </section>
          )}
          {job?.state === "applied" && !pending && (
            <p role="status" className="text-sm text-muted-foreground">
              Changes applied. New terminals and agent sessions use this
              environment.
            </p>
          )}
          {action.error ? (
            <ErrorNotice
              error={action.error}
              onRetry={retry ? () => change(retry) : undefined}
            />
          ) : job?.state === "failed" && job.code ? (
            <ErrorNotice
              error={new APIError(job.code, 0)}
              onRetry={retry ? () => change(retry) : undefined}
            />
          ) : null}
          {hasNetworkProblem && (
            <Button size="sm" variant="outline" onClick={onSettings}>
              Connection settings
            </Button>
          )}
          <Accordion.Root
            type="single"
            collapsible
            defaultValue="tools"
            className="space-y-3"
          >
            <EnvironmentPanel
              value="tools"
              title="Software tools"
              icon={Package}
              summary={`${inventory.tools.length} configured${inventory.tools_need_sync ? " · Sync needed" : ""}`}
            >
              <SetupTools
                inventory={inventory}
                locked={locked}
                isBusy={isBusy}
                change={change}
              />
            </EnvironmentPanel>
            {inventory.skills_target && (
              <EnvironmentPanel
                value="skills"
                title="Agent skills"
                icon={Sparkles}
                summary={`${inventory.skills.length} configured for ${inventory.skills_target}`}
              >
                <SetupSkills
                  inventory={inventory}
                  locked={locked}
                  isBusy={isBusy}
                  change={change}
                />
              </EnvironmentPanel>
            )}
          </Accordion.Root>
        </>
      )}
    </section>
  );
}

function EnvironmentPanel({
  value,
  title,
  icon: Icon,
  summary,
  children,
}: {
  value: string;
  title: string;
  icon: LucideIcon;
  summary: string;
  children: ReactNode;
}) {
  return (
    <Accordion.Item value={value} className="min-w-0 rounded-md border bg-card">
      <Accordion.Header>
        <Accordion.Trigger className="group flex min-h-14 w-full items-center gap-3 rounded-md px-4 py-3 text-left hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:px-5">
          <Icon
            aria-hidden="true"
            className="size-4 shrink-0 text-muted-foreground"
          />
          <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="text-sm font-semibold">{title}</span>
            <span className="text-xs text-muted-foreground">{summary}</span>
          </span>
          <ChevronDown
            aria-hidden="true"
            className="size-4 shrink-0 text-muted-foreground group-data-[state=open]:rotate-180"
          />
        </Accordion.Trigger>
      </Accordion.Header>
      <Accordion.Content
        forceMount
        className="min-w-0 border-t p-4 data-[state=closed]:hidden sm:p-5"
      >
        {children}
      </Accordion.Content>
    </Accordion.Item>
  );
}
