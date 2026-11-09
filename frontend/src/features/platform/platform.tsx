import { snapshotOptions } from "./api";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Boxes,
  Cpu,
  MemoryStick,
  Monitor,
  Network,
  CalendarClock,
  Files,
  ShieldCheck,
  TerminalSquare,
  ArrowUpRight,
} from "lucide-react";
import { ErrorNotice, Observation, State } from "@/shared/components";
import { bytes } from "@/shared/format";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";

export function RuntimeOverview() {
  const query = useQuery(snapshotOptions());
  const usage = query.data?.usage;
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Runtime overview</h1>
          <p>
            Service health and workspace allocations across this deployment.
          </p>
        </div>
        <Observation
          at={query.dataUpdatedAt}
          fetching={query.isFetching}
          error={query.isError}
        />
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      <div className="mb-7 grid divide-y rounded-xl border bg-background sm:grid-cols-3 sm:divide-x sm:divide-y-0">
        {[
          {
            name: "Running workspaces",
            icon: Boxes,
            value: usage
              ? `${usage.running_workspaces ?? "Unknown"} / ${usage.max_running_workspaces ?? "Unknown"}`
              : "—",
            detail: "Current deployment allocation",
          },
          {
            name: "Allocated CPU",
            icon: Cpu,
            value:
              usage?.cpu_allocated == null
                ? "—"
                : `${usage.cpu_allocated} cores`,
            detail: "Reserved for workspace containers",
          },
          {
            name: "Allocated memory",
            icon: MemoryStick,
            value: bytes(usage?.memory_allocated_bytes),
            detail: "Reserved workspace memory",
          },
        ].map((item) => (
          <div key={item.name} className="p-6">
            <div className="mb-4 flex items-center justify-between text-xs font-medium text-muted-foreground">
              <span>{item.name}</span>
              <item.icon className="size-4" />
            </div>
            <div className="text-2xl font-semibold tracking-tight">
              {item.value}
            </div>
            <p className="mt-2 text-xs text-muted-foreground">{item.detail}</p>
          </div>
        ))}
      </div>
      <div className="mb-4 flex items-center gap-2">
        <Activity className="size-4 text-muted-foreground" />
        <h2 className="text-sm font-semibold">Platform services</h2>
      </div>
      {query.isPending ? (
        <Skeleton className="h-72 w-full" />
      ) : (
        <div className="overflow-hidden rounded-xl border bg-background">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40">
                <TableHead className="pl-5">Service</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="pr-5">Observation</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data?.services?.items.map((s) => (
                <TableRow key={s.name}>
                  <TableCell className="py-4 pl-5 font-medium">
                    {s.name}
                  </TableCell>
                  <TableCell>
                    <State value={s.status} />
                  </TableCell>
                  <TableCell className="pr-5 text-xs text-muted-foreground">
                    {s.reason_code?.replaceAll("_", " ").toLowerCase() ||
                      "No diagnostic available"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <div className="mt-5 flex items-start gap-2 text-xs leading-6 text-muted-foreground">
        <ShieldCheck className="mt-1 size-3.5 shrink-0" />
        <p>
          Service health reports availability. Docker mode does not establish
          verified VM isolation or complete network containment.
        </p>
      </div>
    </>
  );
}
const guides = [
  {
    icon: Boxes,
    title: "Create your workspace",
    body: "Choose a project, then create a workspace with T3 and Codex. Select a global connection profile or keep its own network settings.",
  },
  {
    icon: Monitor,
    title: "Connect T3 desktop",
    body: "Use the desktop button on a workspace row, then Open T3 desktop in the pairing dialog. You can also paste its one-use link into desktop Settings → Environments → Add environment. Desktop must run on this Docker host.",
  },
  {
    icon: Network,
    title: "Choose what it can reach",
    body: "Manage global Connection profiles in the sidebar. Assign one through workspace Settings. Allowlist entries are exact host:port pairs. Fully open uses direct networking and DNS; stop the workspace before switching that mode.",
  },
  {
    icon: CalendarClock,
    title: "Schedule development work",
    body: "Create an instruction file such as TASK.md inside the workspace. In Schedules, choose the workspace and file and set a run time. Instructions and results stay in that workspace.",
  },
  {
    icon: Files,
    title: "Share a project file",
    body: "Use Copy a file on the Workspaces page. Transfers are explicit, stay within the project and preserve existing destination files. Workspace history and private agent homes are not copied.",
  },
  {
    icon: TerminalSquare,
    title: "Keep work available",
    body: "Workspace Settings controls automatic idle shutdown. Opening it resets the timer; active runs and tracked background jobs prevent shutdown. Stopping preserves files and conversations.",
  },
];
export function GettingStarted() {
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Getting started</h1>
          <p>From a project boundary to an agent ready for development.</p>
        </div>
      </div>
      <div className="grid gap-6 lg:grid-cols-[1fr_300px]">
        <div className="overflow-hidden rounded-xl border bg-background">
          {guides.map((item) => (
            <section
              key={item.title}
              className="flex items-start gap-4 border-b p-6 last:border-0"
            >
              <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/40 text-muted-foreground">
                <item.icon className="size-4" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">{item.title}</h2>
                <p className="mt-2 max-w-prose text-sm leading-7 text-muted-foreground">
                  {item.body}
                </p>
              </div>
            </section>
          ))}
        </div>
        <aside className="space-y-5">
          <section className="rounded-xl border bg-background p-5">
            <h2 className="text-sm font-semibold">Provider connection</h2>
            <p className="mt-3 text-xs leading-6 text-muted-foreground">
              If credentials are unavailable, run this from the project terminal
              to sync the existing Codex login and repair Vault connectivity.
            </p>
            <pre className="mt-4 whitespace-pre-wrap break-all">
              python3 scripts/subscription_auth.py repair
            </pre>
            <p className="mt-3 text-xs leading-6 text-muted-foreground">
              Expired logins need refreshing through the native provider CLI.
              There is no automatic API-billing fallback.
            </p>
          </section>
          <section className="rounded-xl border bg-background p-5">
            <ShieldCheck className="mb-3 size-5 text-muted-foreground" />
            <h2 className="text-sm font-semibold">One project, one boundary</h2>
            <p className="mt-3 text-xs leading-6 text-muted-foreground">
              Files, conversations and private agent state remain separate
              between projects. Shared connection profiles are configuration,
              not shared workspace storage.
            </p>
          </section>
          <a
            href="https://github.com/pingdotgg/t3code"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-between rounded-lg border bg-background px-4 py-3 text-xs text-muted-foreground hover:text-foreground"
          >
            T3 Code project
            <ArrowUpRight className="size-3.5" />
          </a>
        </aside>
      </div>
    </>
  );
}
