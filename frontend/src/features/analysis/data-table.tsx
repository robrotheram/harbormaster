import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { privateContentOptions } from "@/shared/private-content";
import { DataError, DataNotice } from "./data-state";
import type { DataPage, PreviewRequest, TableSpec } from "./api";

const control =
  "h-9 min-w-0 max-w-full rounded-md border bg-background px-2 focus-visible:outline-2 focus-visible:outline-ring";
type Filter = NonNullable<PreviewRequest["filters"]>[number];

export function DataTable({
  project,
  worker,
  id,
  spec,
  load,
  compact = false,
}: {
  project: string;
  worker: string;
  id: string;
  spec: TableSpec;
  compact?: boolean;
  load: (params: PreviewRequest, signal: AbortSignal) => Promise<DataPage>;
}) {
  const [limit, setLimit] = useState(spec.page_size || 100);
  const [cursors, setCursors] = useState<string[]>([]);
  const [sort, setSort] = useState<PreviewRequest["sort"]>();
  const [filter, setFilter] = useState<Filter>();
  const [filterField, setFilterField] = useState("");
  const [filterOp, setFilterOp] = useState<Filter["op"]>("contains");
  const [filterValue, setFilterValue] = useState("");
  const params = {
    limit,
    sort,
    filters: filter ? [filter] : undefined,
    cursor: cursors.at(-1),
  };
  const query = useQuery({
    ...privateContentOptions,
    queryKey: ["project", project, "analysis-table", worker, id, params],
    queryFn: ({ signal }) => load(params, signal),
    placeholderData: (previous) => previous,
    retry: false,
  });
  const data = query.data;
  const busy = query.isFetching;
  return (
    <div className="min-w-0 space-y-3 text-sm" aria-busy={busy}>
      <div className="flex flex-wrap items-end justify-between gap-3">
        {spec.filterable && (
          <form
            className="flex min-w-0 flex-wrap items-end gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              if (!filterField) return;
              setFilter({
                field: filterField,
                op: filterOp,
                value:
                  filterOp === "contains" || filterOp === "eq"
                    ? filterValue
                    : undefined,
              });
              setCursors([]);
            }}
          >
            <label className="grid min-w-0 gap-1">
              Filter column
              <select
                className={control}
                value={filterField}
                onChange={(e) => setFilterField(e.target.value)}
              >
                <option value="">Choose column</option>
                {data?.columns.map(({ name }) => (
                  <option key={name}>{name}</option>
                ))}
              </select>
            </label>
            <label className="grid min-w-0 gap-1">
              Match
              <select
                className={control}
                value={filterOp}
                onChange={(e) => setFilterOp(e.target.value as Filter["op"])}
              >
                <option value="contains">Contains</option>
                <option value="eq">Equals</option>
                <option value="is_null">Is null</option>
                <option value="is_not_null">Is present</option>
              </select>
            </label>
            {(filterOp === "contains" || filterOp === "eq") && (
              <label className="grid min-w-0 gap-1">
                Value
                <Input
                  value={filterValue}
                  maxLength={512}
                  onChange={(e) => setFilterValue(e.target.value)}
                />
              </label>
            )}
            <Button size="lg" variant="outline" disabled={!filterField || busy}>
              Apply filter
            </Button>
            {filter && (
              <Button
                size="lg"
                type="button"
                variant="ghost"
                onClick={() => {
                  setFilter(undefined);
                  setFilterValue("");
                  setCursors([]);
                }}
              >
                Clear filter
              </Button>
            )}
          </form>
        )}
        <label className="grid min-w-0 gap-1">
          Rows per page
          <select
            className={control}
            value={limit}
            onChange={(e) => {
              setLimit(Number(e.target.value));
              setCursors([]);
            }}
          >
            {[...new Set([25, 50, 100, 250, 500, spec.page_size || 100])]
              .sort((a, b) => a - b)
              .map((n) => (
                <option key={n}>{n}</option>
              ))}
          </select>
        </label>
      </div>
      {filter && (
        <p className="text-muted-foreground">Filtered by {filter.field}.</p>
      )}
      {query.isError ? (
        <DataError
          error={query.error}
          retry={() => {
            if (cursors.length) setCursors([]);
            else void query.refetch();
          }}
        />
      ) : !data ? (
        <p role="status">Loading rows…</p>
      ) : (
        <>
          <DataNotice data={data} />
          <div
            className={
              compact
                ? "max-h-[min(40dvh,26rem)] overflow-auto rounded-md border"
                : "max-h-[30rem] overflow-auto rounded-md border"
            }
            tabIndex={0}
            role="region"
            aria-label="Data rows"
          >
            <table className="w-full text-left">
              <thead className="sticky top-0 bg-muted">
                <tr>
                  {data.columns.map(({ name, type }) => {
                    const direction =
                      sort?.[0]?.field === name ? sort[0].direction : undefined;
                    return (
                      <th
                        className={
                          compact
                            ? "min-w-24 whitespace-nowrap px-3 py-2 align-top font-medium"
                            : "min-w-24 px-3 py-2 align-top font-medium"
                        }
                        key={name}
                        scope="col"
                        aria-sort={
                          direction
                            ? direction === "asc"
                              ? "ascending"
                              : "descending"
                            : undefined
                        }
                      >
                        {spec.sortable ? (
                          <button
                            type="button"
                            className="text-left hover:underline"
                            onClick={() => {
                              setSort(
                                direction === "desc"
                                  ? undefined
                                  : [
                                      {
                                        field: name,
                                        direction:
                                          direction === "asc" ? "desc" : "asc",
                                      },
                                    ],
                              );
                              setCursors([]);
                            }}
                          >
                            {name}
                            {direction === "asc"
                              ? " ↑"
                              : direction === "desc"
                                ? " ↓"
                                : ""}
                          </button>
                        ) : (
                          name
                        )}
                        <span className="block text-xs font-normal text-muted-foreground">
                          {type}
                        </span>
                      </th>
                    );
                  })}
                </tr>
              </thead>
              <tbody className="divide-y">
                {data.rows.map((row, i) => (
                  <tr key={i}>
                    {row.map((value, j) => (
                      <td
                        key={j}
                        className={
                          compact
                            ? "max-w-80 truncate px-3 py-1.5 align-top tabular-nums"
                            : "max-w-80 px-3 py-2 align-top tabular-nums [overflow-wrap:anywhere]"
                        }
                        title={
                          compact && value !== null ? String(value) : undefined
                        }
                      >
                        {value === null ? (
                          <i className="text-muted-foreground">null</i>
                        ) : (
                          String(value)
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
            {data.rows.length === 0 && (
              <p className="p-3 text-muted-foreground">No matching rows.</p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={!cursors.length || busy}
              onClick={() => setCursors(cursors.slice(0, -1))}
            >
              Previous
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!data.next_cursor || busy}
              onClick={() => {
                if (data.next_cursor)
                  setCursors([...cursors, data.next_cursor]);
              }}
            >
              Next
            </Button>
            <span role="status" className="text-muted-foreground">
              {busy
                ? "Loading rows…"
                : `Page ${cursors.length + 1} · ${data.rows.length} rows`}
            </span>
          </div>
        </>
      )}
    </div>
  );
}
