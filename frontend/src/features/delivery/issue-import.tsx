import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import { boardKey } from "./board-api";
import { workKeys } from "@/features/work/api";

type Connection = { id: string; repository: string; host: string };
type Issue = { number: number; title: string; state: string };
export function IssueImport({
  project,
  onClose,
}: {
  project: string;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [connection, setConnection] = useState("");
  const [page, setPage] = useState(1);
  const [chosen, setChosen] = useState<number[]>([]);
  const [results, setResults] = useState<{ number: number; error?: unknown }[]>(
    [],
  );
  const connections = useQuery({
    queryKey: ["platform", "forge-connections"],
    queryFn: () => request<{ items: Connection[] }>("/forge-connections"),
  });
  const issues = useQuery({
    queryKey: ["platform", "forge-issues", connection, page],
    enabled: !!connection,
    queryFn: ({ signal }) =>
      request<{ items: Issue[] }>(
        `/forge-connections/${connection}/issues?page=${page}`,
        undefined,
        signal,
      ),
    gcTime: 0,
  });
  const imports = useMutation({
    mutationFn: async () => {
      const outcomes: { number: number; error?: unknown }[] = [];
      for (const number of chosen) {
        try {
          await request(
            `/forge-connections/${connection}/issues/${number}/import`,
            { project_id: project },
          );
          outcomes.push({ number });
        } catch (error) {
          outcomes.push({ number, error });
        }
      }
      setResults(outcomes);
      setChosen(outcomes.filter((o) => o.error).map((o) => o.number));
      await client.invalidateQueries({ queryKey: boardKey(project) });
      await client.invalidateQueries({ queryKey: workKeys.list(project) });
    },
  });
  return (
    <section className="board-import" aria-label="Import repository issues">
      <div className="board-panel-heading">
        <div>
          <h2>Bring repository issues onto this board</h2>
          <p>
            Choose the issues to track here. Importing does not start a worker.
          </p>
        </div>
        <Button variant="ghost" disabled={imports.isPending} onClick={onClose}>
          Close
        </Button>
      </div>
      <label className="block max-w-lg text-sm">
        Repository
        <NativeSelect
          aria-label="Repository to import from"
          value={connection}
          disabled={imports.isPending}
          onChange={(e) => {
            setConnection(e.target.value);
            setPage(1);
            setChosen([]);
            setResults([]);
          }}
        >
          <option value="">Choose a connected repository</option>
          {connections.data?.items.map((c) => (
            <option key={c.id} value={c.id}>
              {c.repository} ({c.host})
            </option>
          ))}
        </NativeSelect>
      </label>
      {connections.isError && <ErrorNotice error={connections.error} />}
      {connections.data?.items.length === 0 && (
        <p>
          No repositories connected yet. Add one under Platform → Repositories.
        </p>
      )}
      {issues.isError && <ErrorNotice error={issues.error} />}{" "}
      {connection && issues.isPending && (
        <p role="status">Loading repository issues…</p>
      )}
      <div className="board-import-list">
        {issues.data?.items.map((i) => (
          <label key={i.number}>
            <input
              type="checkbox"
              disabled={imports.isPending}
              checked={chosen.includes(i.number)}
              onChange={(e) =>
                setChosen(
                  e.target.checked
                    ? [...chosen, i.number]
                    : chosen.filter((n) => n !== i.number),
                )
              }
            />
            <span className="text-muted-foreground">#{i.number}</span>
            <strong>{i.title}</strong>
            <span className="text-muted-foreground">{i.state}</span>
          </label>
        ))}
      </div>
      {!!connection && (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            disabled={page === 1 || issues.isFetching || imports.isPending}
            onClick={() => {
              setPage(page - 1);
              setChosen([]);
            }}
          >
            Previous
          </Button>
          <span className="text-xs">Page {page}</span>
          <Button
            variant="outline"
            disabled={!issues.data || issues.isFetching || imports.isPending}
            onClick={() => {
              setPage(page + 1);
              setChosen([]);
            }}
          >
            Next
          </Button>
          <Button
            disabled={!chosen.length || imports.isPending}
            onClick={() => imports.mutate()}
          >
            {imports.isPending
              ? "Importing…"
              : `Import selected (${chosen.length})`}
          </Button>
        </div>
      )}
      {!!results.length && (
        <p role="status">
          {results.filter((r) => !r.error).length} issues added or already on
          this board.
        </p>
      )}
      {results
        .filter((r) => r.error)
        .map((r) => (
          <div key={r.number}>
            <p>Issue #{r.number}</p>
            <ErrorNotice error={r.error} />
          </div>
        ))}
    </section>
  );
}
