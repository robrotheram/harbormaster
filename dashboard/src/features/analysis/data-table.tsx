import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  columnVisibilityFeature,
  rowSortingFeature,
  tableFeatures,
  useTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { ArrowDown, ArrowUp, ArrowUpDown, Columns3 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { privateContentOptions } from "@/shared/private-content";
import { DataError, DataNotice } from "./data-state";
import type { Cell, DataPage, PreviewRequest, TableSpec } from "./api";

type Filter = NonNullable<PreviewRequest["filters"]>[number];

const features = tableFeatures({ columnVisibilityFeature, rowSortingFeature });
const emptyRows: Cell[][] = [];
// Keep arbitrary dataset names (including "__proto__") safe as table state keys.
const fieldPrefix = "field:";

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
  const initialLimit = spec.page_size || 100;
  const [limit, setLimit] = useState(initialLimit);
  const [cursors, setCursors] = useState<string[]>([]);
  const [sorting, setSorting] = useState<SortingState>([]);
  const [filter, setFilter] = useState<Filter>();
  const [filterField, setFilterField] = useState("");
  const [filterOp, setFilterOp] = useState<Filter["op"]>("contains");
  const [filterValue, setFilterValue] = useState("");
  const params: PreviewRequest = {
    limit,
    sort: sorting.length
      ? sorting.map(({ id, desc }) => ({
          field: id.slice(fieldPrefix.length),
          direction: desc ? "desc" : "asc",
        }))
      : undefined,
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
  const schema = data?.columns;
  const field = filterField || filter?.field || schema?.[0]?.name || "";
  const needsValue = filterOp === "contains" || filterOp === "eq";

  const columns = useMemo(
    (): ColumnDef<typeof features, Cell[]>[] =>
      (schema ?? []).map(({ name, type }, index) => ({
        id: fieldPrefix + name,
        accessorFn: (row) => row[index],
        header: ({ column }) => {
          const direction = column.getIsSorted();
          const SortIcon =
            direction === "asc"
              ? ArrowUp
              : direction === "desc"
                ? ArrowDown
                : ArrowUpDown;
          return (
            <>
              {column.getCanSort() ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="-ml-1.5 h-7 max-w-full justify-start px-1.5 text-sm"
                  onClick={column.getToggleSortingHandler()}
                >
                  <span className="truncate" title={name}>
                    {name}
                  </span>
                  <SortIcon
                    aria-hidden="true"
                    className={cn(
                      "size-3.5",
                      !direction && "text-muted-foreground",
                    )}
                  />
                </Button>
              ) : (
                <span className="block truncate py-1" title={name}>
                  {name}
                </span>
              )}
              <span className="block text-xs font-normal text-muted-foreground">
                {type}
              </span>
            </>
          );
        },
      })),
    [schema],
  );
  const table = useTable({
    features,
    columns,
    data: data?.rows ?? emptyRows,
    state: { sorting },
    onSortingChange: (updater) => {
      setSorting(updater);
      setCursors([]);
    },
    manualSorting: true,
    enableSorting: !!spec.sortable,
    enableMultiSort: false,
    sortDescFirst: false,
  });
  const visibleCount = table.getVisibleLeafColumns().length;
  const nextCursor = data?.next_cursor;

  return (
    <div className="min-w-0 space-y-2 text-sm" aria-busy={busy}>
      <div className="flex flex-wrap items-center gap-2 empty:hidden">
        {spec.filterable && (
          <form
            className="flex min-w-0 flex-wrap items-center gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              if (!field) return;
              setFilter({
                field,
                op: filterOp,
                value: needsValue ? filterValue : undefined,
              });
              setCursors([]);
            }}
          >
            <NativeSelect
              aria-label="Filter column"
              size="sm"
              wrapperClassName="w-36 max-w-full"
              value={field}
              disabled={!schema?.length}
              onChange={(event) => setFilterField(event.target.value)}
            >
              {!schema?.length && (
                <option value={field}>{field || "Column"}</option>
              )}
              {schema?.map(({ name }) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </NativeSelect>
            <NativeSelect
              aria-label="Match"
              size="sm"
              wrapperClassName="w-auto max-w-full"
              value={filterOp}
              onChange={(event) =>
                setFilterOp(event.target.value as Filter["op"])
              }
            >
              <option value="contains">Contains</option>
              <option value="eq">Equals</option>
              <option value="is_null">Is null</option>
              <option value="is_not_null">Is present</option>
            </NativeSelect>
            {needsValue && (
              <Input
                aria-label="Value"
                placeholder="Filter value…"
                className="h-8 w-48 max-w-full"
                value={filterValue}
                maxLength={512}
                onChange={(event) => setFilterValue(event.target.value)}
              />
            )}
            <Button type="submit" variant="outline" disabled={!field}>
              Apply filter
            </Button>
            {filter && (
              <Button
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
        {columns.length > 1 && (
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="outline" className="ml-auto">
                <Columns3 /> Columns
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="max-w-72">
              {table.getAllLeafColumns().map((column) => (
                <DropdownMenuCheckboxItem
                  key={column.id}
                  checked={column.getIsVisible()}
                  disabled={visibleCount === 1 && column.getIsVisible()}
                  onCheckedChange={(checked) =>
                    column.toggleVisibility(checked)
                  }
                  onSelect={(event) => event.preventDefault()}
                  className="[overflow-wrap:anywhere]"
                >
                  {column.id.slice(fieldPrefix.length)}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      {filter && (
        <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">
          Filtered by {filter.field}.
        </p>
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
          <Table
            containerProps={{
              className: cn(
                "overflow-auto rounded-md border",
                compact ? "max-h-[min(40dvh,26rem)]" : "max-h-[30rem]",
              ),
              tabIndex: 0,
              role: "region",
              "aria-label": "Data rows",
            }}
          >
            <TableHeader className="sticky top-0 z-10 bg-muted">
              {table.getHeaderGroups().map((group) => (
                <TableRow key={group.id} className="hover:bg-transparent">
                  {group.headers.map((header) => {
                    const direction = header.column.getIsSorted();
                    return (
                      <TableHead
                        key={header.id}
                        scope="col"
                        className="h-auto min-w-24 max-w-80 px-3 py-1.5 align-top"
                        aria-sort={
                          direction
                            ? direction === "asc"
                              ? "ascending"
                              : "descending"
                            : undefined
                        }
                      >
                        <table.FlexRender header={header} />
                      </TableHead>
                    );
                  })}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody>
              {table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {row.getVisibleCells().map((cell) => {
                    const value = cell.getValue();
                    return (
                      <TableCell
                        key={cell.id}
                        className={cn(
                          "max-w-80 px-3 align-top tabular-nums",
                          compact
                            ? "truncate py-1.5"
                            : "py-2 whitespace-normal [overflow-wrap:anywhere]",
                        )}
                        title={
                          compact && value !== null ? String(value) : undefined
                        }
                      >
                        {value === null ? (
                          <i className="text-muted-foreground">null</i>
                        ) : (
                          String(value)
                        )}
                      </TableCell>
                    );
                  })}
                </TableRow>
              ))}
              {!data.rows.length && (
                <TableRow>
                  <TableCell
                    colSpan={Math.max(visibleCount, 1)}
                    className="h-20 text-center text-muted-foreground"
                  >
                    {filter ? "No matching rows." : "No rows to display."}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <span role="status" className="text-muted-foreground">
              {busy
                ? "Loading rows…"
                : `Page ${cursors.length + 1} · ${data.rows.length} ${data.rows.length === 1 ? "row" : "rows"}`}
            </span>
            <div className="flex flex-wrap items-center gap-2">
              <label className="flex items-center gap-2 text-muted-foreground">
                Rows per page
                <NativeSelect
                  size="sm"
                  wrapperClassName="w-auto"
                  value={limit}
                  onChange={(event) => {
                    setLimit(Number(event.target.value));
                    setCursors([]);
                  }}
                >
                  {[...new Set([25, 50, 100, 250, 500, initialLimit])]
                    .sort((a, b) => a - b)
                    .map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                </NativeSelect>
              </label>
              <Button
                type="button"
                variant="outline"
                disabled={!cursors.length || busy}
                onClick={() => setCursors(cursors.slice(0, -1))}
              >
                Previous
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={!nextCursor || busy}
                onClick={() => {
                  if (nextCursor) setCursors([...cursors, nextCursor]);
                }}
              >
                Next
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
