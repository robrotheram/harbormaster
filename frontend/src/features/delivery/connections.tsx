import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice, State, Modal } from "@/shared/components";
import { request } from "@/shared/http";
import type { Project } from "@/shared/types";
import { evidenceURL, workKeys, type WorkItem } from "@/features/work/api";

import { boardKey } from "./board-api";
import { AccountForm, type ForgeAccount } from "./account-form";
type Connection = {
  id: string;
  host: string;
  repository: string;
};
type Issue = {
  number: number;
  title: string;
  body: string;
  url: string;
  state: string;
  labels: string[];
  milestone: string;
};
export function RepositoryConnections({
  projects,
  embedded = false,
}: {
  projects: Project[];
  embedded?: boolean;
}) {
  const client = useQueryClient();
  const [host, setHost] = useState("");
  const [accountForm, setAccountForm] = useState<ForgeAccount | "new" | null>(
    null,
  );
  const [accountSaved, setAccountSaved] = useState("");
  const [repo, setRepo] = useState("");
  const [selected, setSelected] = useState("");
  const [disconnecting, setDisconnecting] = useState<Connection>();
  const hosts = useQuery({
    queryKey: ["forge-hosts"],
    queryFn: () => request<{ items: ForgeAccount[] }>("/forge-hosts"),
  });
  const key = ["platform", "forge-connections"];
  const connections = useQuery({
    queryKey: key,
    queryFn: () => request<{ items: Connection[] }>("/forge-connections"),
  });
  const save = useMutation({
    mutationFn: () =>
      request("/forge-connections", {
        host,
        repository: repo.trim(),
      }),
    onSuccess: async () => {
      setRepo("");
      await client.invalidateQueries({ queryKey: key });
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) => request(`/forge-connections/${id}/delete`, {}),
    onSuccess: async () => {
      setSelected("");
      setDisconnecting(undefined);
      await client.invalidateQueries({ queryKey: key });
    },
  });
  const connection = connections.data?.items.find((c) => c.id === selected);
  const canConnect =
    !save.isPending &&
    !!repo.trim() &&
    hosts.isSuccess &&
    !!hosts.data?.items.some((account) => account.name === host);
  return (
    <div className="space-y-5">
      <div className="page-title">
        <div>
          {embedded ? (
            <h2 className="text-lg font-semibold">Repository connections</h2>
          ) : (
            <h1>Repository connections</h1>
          )}
        </div>
      </div>
      {save.isError && <ErrorNotice error={save.error} />}
      {hosts.isPending && <p role="status">Loading configured accounts…</p>}
      {accountSaved && (
        <p role="status" className="text-sm">
          Account {accountSaved} verified and saved. Choose a repository to
          connect below.
        </p>
      )}
      {accountForm !== null ? (
        <AccountForm
          key={typeof accountForm === "string" ? "new" : accountForm.name}
          account={accountForm === "new" ? undefined : accountForm}
          onCancel={() => setAccountForm(null)}
          onSaved={async (account) => {
            await client.invalidateQueries({ queryKey: ["forge-hosts"] });
            setHost(account.name);
            setAccountSaved(account.name);
            setAccountForm(null);
          }}
        />
      ) : (
        <section className="space-y-3 rounded-md border p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="font-semibold">Provider accounts</h2>
              <p className="text-sm text-muted-foreground">
                Configure GitHub, GitLab.com or your private GitLab server.
              </p>
            </div>
            <Button
              onClick={() => {
                setAccountSaved("");
                setAccountForm("new");
              }}
            >
              Add account
            </Button>
          </div>
          {hosts.isError && (
            <ErrorNotice
              error={hosts.error}
              onRetry={() => void hosts.refetch()}
              retrying={hosts.isFetching}
            />
          )}
          {hosts.data?.items.length === 0 && (
            <p className="text-sm text-muted-foreground">
              Add your first account to connect a repository.
            </p>
          )}
          {hosts.data?.items.map((account) => (
            <div
              key={account.name}
              className="flex flex-wrap items-center justify-between gap-3 border-t pt-3 text-sm"
            >
              <div className="min-w-0 break-words">
                <strong>{account.name}</strong>
                <p className="break-all text-xs text-muted-foreground">
                  {account.api_url}
                </p>
              </div>
              {account.managed ? (
                <Button
                  variant="outline"
                  onClick={() => setAccountForm(account)}
                >
                  Update token
                </Button>
              ) : (
                <span className="text-xs text-muted-foreground">
                  Configured by operator
                </span>
              )}
            </div>
          ))}
        </section>
      )}
      {!!hosts.data?.items.length && (
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (canConnect) save.mutate();
          }}
        >
          <fieldset
            disabled={save.isPending}
            className="flex min-w-0 w-full flex-wrap items-end gap-3"
          >
            <label className="max-w-full min-w-0 space-y-1 text-sm">
              Account
              <NativeSelect
                aria-label="Account"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                required
              >
                <option value="">Select an account</option>
                {hosts.data?.items.map((h) => (
                  <option key={h.name} value={h.name}>
                    {h.name} ({h.provider})
                  </option>
                ))}
              </NativeSelect>
            </label>
            <label className="min-w-64 flex-1 space-y-1 text-sm">
              Repository
              <Input
                value={repo}
                onChange={(e) => setRepo(e.target.value)}
                placeholder="owner/repository or group/subgroup/project"
                required
                maxLength={500}
              />
            </label>
            <Button disabled={!canConnect}>
              {save.isPending ? "Checking access…" : "Connect repository"}
            </Button>
          </fieldset>
        </form>
      )}
      <div className="space-y-2">
        <h2 className="font-semibold">Connected repositories</h2>
        {connections.isError && (
          <ErrorNotice
            error={connections.error}
            onRetry={() => void connections.refetch()}
            retrying={connections.isFetching}
          />
        )}
        {connections.data?.items.map((c) => (
          <div
            key={c.id}
            className="flex flex-wrap items-center justify-between gap-3 rounded border p-3"
          >
            <button
              className="min-w-0 flex-1 break-all text-left text-sm hover:underline"
              aria-label={`Browse ${c.repository} tickets`}
              onClick={() => setSelected(c.id)}
              aria-pressed={selected === c.id}
            >
              <strong>{c.repository}</strong>
              <span className="ml-3 text-muted-foreground">{c.host}</span>
            </button>
            <Button
              variant="outline"
              disabled={remove.isPending}
              aria-label={`Disconnect ${c.repository}`}
              onClick={() => {
                remove.reset();
                setDisconnecting(c);
              }}
            >
              Disconnect
            </Button>
          </div>
        ))}
      </div>
      {disconnecting && (
        <Modal
          title="Disconnect repository?"
          onClose={() => {
            if (!remove.isPending) setDisconnecting(undefined);
          }}
          closeOnEscape={!remove.isPending}
          closeDisabled={remove.isPending}
        >
          <p className="break-words text-sm">
            Disconnect <strong>{disconnecting.repository}</strong> (
            {disconnecting.host}) from Workbench?
          </p>
          <p className="text-sm text-muted-foreground">
            Imported work remains in its project, but its repository sync will
            be unavailable. This does not delete the repository or its tickets.
          </p>
          {remove.isError && <ErrorNotice error={remove.error} />}
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              variant="outline"
              disabled={remove.isPending}
              onClick={() => setDisconnecting(undefined)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => remove.mutate(disconnecting.id)}
            >
              {remove.isPending ? "Disconnecting…" : "Disconnect repository"}
            </Button>
          </div>
        </Modal>
      )}
      {connection ? (
        <Tickets
          key={connection.id}
          connection={connection}
          projects={projects}
        />
      ) : (
        <p className="text-sm text-muted-foreground">
          {connections.isPending
            ? "Loading repository connections…"
            : connections.isError
              ? "Repository connections could not load. Use Try again above to reload them."
              : connections.data?.items.length
                ? "Select a connected repository to browse its tickets."
                : hosts.data?.items.length
                  ? "Choose an account and enter a repository above to connect your first repository."
                  : "After adding an account, you can connect a repository and import its issues into a project."}
        </p>
      )}
    </div>
  );
}
function Tickets({
  connection: c,
  projects,
}: {
  connection: Connection;
  projects: Project[];
}) {
  const [project, setProject] = useState("");
  const client = useQueryClient();
  const [page, setPage] = useState(1);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [imported, setImported] = useState<WorkItem>();
  const base = `/forge-connections/${c.id}/issues`;
  const key = ["platform", "forge-issues", c.id, page];
  const issues = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      request<{ items: Issue[] }>(`${base}?page=${page}`, undefined, signal),
    gcTime: 0,
  });
  const action = useMutation({
    mutationFn: async ({ op, issue }: { op: string; issue?: Issue }) => {
      if (op === "import") {
        if (!project) throw new Error("Choose a destination project.");
        const item = await request<WorkItem>(
          `${base}/${issue!.number}/import`,
          { project_id: project },
        );
        setImported(item);
        await client.invalidateQueries({
          queryKey: workKeys.list(item.project_id),
        });
        await client.invalidateQueries({ queryKey: boardKey(item.project_id) });
      }
      if (op === "state")
        await request(`${base}/${issue!.number}`, {
          state: issue!.state === "closed" ? "open" : "closed",
        });
      if (op === "create") {
        await request(base, { title, body });
        setTitle("");
        setBody("");
      }
    },
    onSuccess: () => client.invalidateQueries({ queryKey: key }),
  });
  return (
    <section className="space-y-4 border-t pt-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-semibold">Tickets / {c.repository}</h2>
        <Button
          variant="outline"
          disabled={issues.isFetching}
          onClick={() => issues.refetch()}
        >
          Refresh tickets
        </Button>
      </div>
      <label className="block max-w-sm space-y-1 text-sm">
        Import into project
        <NativeSelect
          value={project}
          onChange={(e) => setProject(e.target.value)}
          disabled={action.isPending}
        >
          <option value="">Choose a destination project</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </NativeSelect>
      </label>
      <details>
        <summary className="cursor-pointer text-sm">Create a ticket</summary>
        <form
          className="mt-3 space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!action.isPending && title.trim())
              action.mutate({ op: "create" });
          }}
        >
          <label className="block text-sm">
            Title
            <Input
              value={title}
              disabled={action.isPending}
              onChange={(e) => setTitle(e.target.value)}
              required
              maxLength={256}
            />
          </label>
          <label className="block text-sm">
            Description
            <Textarea
              value={body}
              disabled={action.isPending}
              onChange={(e) => setBody(e.target.value)}
              maxLength={65536}
            />
          </label>
          <Button disabled={action.isPending || !title.trim()}>
            Create ticket
          </Button>
        </form>
      </details>
      {issues.isPending && <p role="status">Loading tickets…</p>}
      {issues.isError && (
        <ErrorNotice
          error={issues.error}
          onRetry={() => void issues.refetch()}
          retrying={issues.isFetching}
        />
      )}{" "}
      {action.isError && <ErrorNotice error={action.error} />}
      {imported && (
        <p role="status" className="text-sm">
          Imported as a planned work item.{" "}
          <Link
            className="text-primary underline"
            to="/projects/$projectId/$section"
            params={{ projectId: imported.project_id, section: "work" }}
            search={{ work: imported.id }}
          >
            Open work to start it
          </Link>
        </p>
      )}
      {issues.data?.items.map((issue) => (
        <article key={issue.number} className="space-y-2 border-b py-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-muted-foreground">
              #{issue.number}
            </span>
            <h3 className="font-medium">{issue.title}</h3>
            <State value={issue.state} />
          </div>
          <p className="text-xs text-muted-foreground">
            {[...issue.labels, issue.milestone].filter(Boolean).join(" / ")}
          </p>
          <details>
            <summary className="cursor-pointer text-xs">Description</summary>
            <p className="mt-2 whitespace-pre-wrap break-words text-sm">
              {issue.body || "No description."}
            </p>
          </details>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              disabled={action.isPending || !project}
              onClick={() => action.mutate({ op: "import", issue })}
            >
              Import to factory
            </Button>
            <Button
              variant="outline"
              disabled={action.isPending}
              onClick={() => action.mutate({ op: "state", issue })}
            >
              {issue.state === "closed" ? "Reopen ticket" : "Close ticket"}
            </Button>
            {evidenceURL(issue.url) && (
              <a
                className="text-xs text-primary underline"
                href={evidenceURL(issue.url)}
                target="_blank"
                rel="noreferrer"
              >
                Open in provider
              </a>
            )}
          </div>
        </article>
      ))}
      {issues.data && !issues.data.items.length && (
        <p className="text-sm text-muted-foreground">
          No tickets on this page.
        </p>
      )}
      <div className="flex items-center gap-3">
        <Button
          variant="outline"
          disabled={page === 1 || issues.isFetching}
          onClick={() => setPage(page - 1)}
        >
          Previous
        </Button>
        <span className="text-xs">Page {page}</span>
        <Button
          variant="outline"
          disabled={issues.isFetching || !issues.data?.items.length}
          onClick={() => setPage(page + 1)}
        >
          Next
        </Button>
      </div>
    </section>
  );
}
