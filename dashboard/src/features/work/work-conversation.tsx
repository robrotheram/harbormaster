import { usePersonalWork, useLayoutPreference } from "./personal-work";
import { AgentSelection } from "./agent-selection";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { conversationTitle } from "./conversation-title";
import { useThreadTitles } from "./thread-titles";
import {
  useState,
  useEffect,
  type CSSProperties,
  useCallback,
  useMemo,
  useRef,
  useSyncExternalStore,
  type SetStateAction,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import {
  Plus,
  Search,
  MessageSquare,
  MessagesSquare,
  ChevronDown,
  FlaskConical,
  Globe,
  Monitor,
  Newspaper,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/dropdown-menu";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice, Form, useRequestKey } from "@/shared/components";
import { date, label } from "@/shared/format";
import type { Project } from "@/shared/types";
import { workspaceOptions } from "@/features/workspaces/api";
import { WorkConversation } from "./work-thread";
import { workApi, workKeys, workOptions, type WorkItem } from "./api";
import { startsInWorkspace, workProfiles, type WorkProfile } from "./profiles";
import { stages, workStage, workStatus, type WorkStage } from "./stages";

const webThread = { label: "Web", Icon: Globe };
const researchThread = { label: "Research", Icon: FlaskConical };
const threadTypes = {
  software: webThread,
  desktop: { label: "Desktop", Icon: Monitor },
  research: researchThread,
  "data-science": researchThread,
  digest: { label: "Digest", Icon: Newspaper },
} satisfies Record<WorkItem["kind"], typeof webThread>;

const minRailWidth = 200;
const maxRailWidth = 280;
const clampRailWidth = (width: number) =>
  Math.max(minRailWidth, Math.min(maxRailWidth, width));

function useThreadDrawer(hasConversation: boolean) {
  const media = useMemo(
    () =>
      window.matchMedia(
        hasConversation
          ? "(width < 1600px), (width <= 1920px) and (height <= 920px)"
          : "(width < 1200px)",
      ),
    [hasConversation],
  );
  const subscribe = useCallback(
    (onChange: () => void) => {
      media.addEventListener("change", onChange);
      return () => media.removeEventListener("change", onChange);
    },
    [media],
  );
  return useSyncExternalStore(subscribe, () => media.matches);
}

export function Work({ project }: { project: Project }) {
  return <ProjectConversations key={project.id} project={project} />;
}
function ProjectConversations({ project }: { project: Project }) {
  const navigate = useNavigate();
  const search = useSearch({ strict: false });
  const personal = usePersonalWork();
  const selected = search.work ?? personal.lastThread;
  const [railWidth, setRailWidth] = useLayoutPreference<number>(
    "thread-width",
    252,
  );
  const [panel, setPanel] = useLayoutPreference<"shown" | "hidden">(
    "thread-panel",
    "shown",
  );
  const usesDrawer = useThreadDrawer(Boolean(selected));
  const compactRail = usesDrawer || panel === "hidden";
  const hidePanelButton = useRef<HTMLButtonElement>(null);
  const showPanelButton = useRef<HTMLButtonElement>(null);
  const rail = useRef<HTMLDivElement>(null);
  useEffect(() => {
    personal.setLastThread(selected);
  }, [selected]);
  const query = useQuery(workOptions(project.id));
  const [filter, setFilter] = useState("");
  const [visibleLimit, setVisibleLimit] = useState(20);
  const [stage, setStage] = useState<WorkStage>("all");
  const [worklist, setWorklist] = useState(false);
  const { drafts, setDrafts } = personal;
  const setThreadDraft = useCallback(
    (value: SetStateAction<string>) => {
      setDrafts((old) => ({
        ...old,
        [selected]:
          typeof value === "function" ? value(old[selected] || "") : value,
      }));
    },
    [selected],
  );
  const select = (id: string, openWorkspace = false) => {
    setWorklist(false);
    navigate({
      to: "/projects/$projectId/$section",
      params: { projectId: project.id, section: "work" },
      search: { work: id, view: openWorkspace ? "workspace" : undefined },
    });
  };
  const order = useRef<string[]>([]);
  // Planning records without an execution context belong in Overview.
  const loaded = (query.data || [])
    .filter(
      (item) => !(item.workspace_id === "" && item.specification_path === ""),
    )
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  for (const item of loaded)
    if (!order.current.includes(item.id)) order.current.push(item.id);
  const items = loaded.sort(
    (a, b) => order.current.indexOf(a.id) - order.current.indexOf(b.id),
  );
  const term = filter.trim().toLowerCase();
  const eligible = items.filter(
    (item) => stage === "all" || workStage(item.state) === stage,
  );
  const selectedItem = items.find((item) => item.id === selected);
  const titleItems = [
    ...new Map(
      [
        ...(selectedItem ? [selectedItem] : []),
        ...(term ? eligible : eligible.slice(0, visibleLimit)),
      ].map((item) => [item.id, item]),
    ).values(),
  ];
  const titleState = useThreadTitles(project.id, titleItems);
  const titles = titleState.titles;
  const matches = eligible.filter((i) =>
    [
      titles.get(i.id) || conversationTitle(i),
      i.specification_path,
      i.id,
      i.kind,
      threadTypes[i.kind]?.label || "",
      workProfiles.find((profile) => profile.kind === i.kind)?.label || "",
      titleState.owners.get(i.id) || "",
    ].some((v) => v.toLowerCase().includes(term)),
  );
  const visible = matches.slice(0, visibleLimit);
  const stageLabel = (id: WorkStage) => {
    const label =
      id === "all"
        ? "All threads"
        : stages.find((entry) => entry.id === id)!.label;
    const count =
      query.isPending || query.isError
        ? "—"
        : id === "all"
          ? items.length
          : items.filter((item) => workStage(item.state) === id).length;
    return `${label} (${count})`;
  };
  const togglePanel = () => {
    const nextFocus = compactRail ? hidePanelButton : showPanelButton;
    setPanel(compactRail ? "shown" : "hidden");
    requestAnimationFrame(() => nextFocus.current?.focus());
  };
  const renderThread = (item: WorkItem, compact = false) => {
    const title = titles.get(item.id) || conversationTitle(item);
    const owner = titleState.owners.get(item.id);
    const { label: type, Icon } = threadTypes[item.kind] ?? {
      label: label(item.kind),
      Icon: MessageSquare,
    };
    const status = workStatus(item.state);
    const hasDraft = !!drafts[item.id]?.trim();
    const details = [type, label(status), hasDraft && "Unsent draft"]
      .filter(Boolean)
      .join(". ");
    const icon = (
      <span className="thread-type-icon" aria-hidden="true">
        <Icon size={15} />
      </span>
    );
    const rowDetails = (
      <>
        <span className="thread-list-content">
          <span className="conversation-work-name">{title}</span>
          {(owner || hasDraft) && (
            <span className="conversation-work-meta">
              {hasDraft && <span className="thread-draft">Draft</span>}
              {owner && (
                <span className="truncate" title={owner}>
                  {owner}
                </span>
              )}
            </span>
          )}
        </span>
        <span className="thread-list-trailing">
          <time dateTime={item.created_at} title={date(item.created_at)}>
            {new Date(item.created_at).toLocaleDateString(undefined, {
              month: "short",
              day: "numeric",
            })}
          </time>
          <span
            className="thread-state"
            data-stage={workStage(item.state)}
            data-state={status}
            aria-hidden="true"
          />
        </span>
      </>
    );
    const button = (
      <button
        key={item.id}
        className={`conversation-work-item ${compact ? "" : "thread-row"} ${selected === item.id ? "is-selected" : ""}`}
        onClick={() => select(item.id)}
        aria-label={compact ? title : undefined}
        aria-pressed={selected === item.id}
        aria-description={details}
        title={
          compact
            ? undefined
            : [
                title,
                details,
                date(item.created_at),
                title === "Untitled conversation" && item.id,
              ]
                .filter(Boolean)
                .join("\n")
        }
      >
        {icon}
        {!compact && rowDetails}
      </button>
    );
    return compact ? (
      <Tooltip key={item.id}>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent
          side="right"
          sideOffset={8}
          className="thread-tooltip w-64 max-w-xs border border-border bg-popover p-0 text-popover-foreground"
          aria-label={[
            details,
            owner && `Assigned to ${owner}`,
            date(item.created_at),
          ]
            .filter(Boolean)
            .join(". ")}
        >
          <div className="thread-row">
            {icon}
            {rowDetails}
          </div>
        </TooltipContent>
      </Tooltip>
    ) : (
      button
    );
  };
  const renderWorkList = (docked = false) => (
    <aside className="conversation-worklist" aria-label="Threads">
      <div className="thread-list-actions">
        <Button className="h-9 flex-1" onClick={() => select("")}>
          <Plus size={14} /> New thread
        </Button>
        {docked && (
          <Button
            ref={hidePanelButton}
            className="size-9"
            variant="outline"
            size="icon"
            aria-label="Hide thread panel"
            title="Hide thread panel"
            aria-expanded="true"
            onClick={togglePanel}
          >
            <MessagesSquare size={16} />
          </Button>
        )}
      </div>
      <div className="conversation-search">
        <Search size={14} />
        <Input
          className="h-9 pl-8 text-xs"
          aria-label="Search threads"
          placeholder="Find a thread…"
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
            setVisibleLimit(20);
          }}
        />
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            aria-label={`Filter threads by status: ${stageLabel(stage)}`}
            className="conversation-status-filter h-9 w-full justify-between font-normal"
          >
            <span>{stageLabel(stage)}</span>
            <ChevronDown size={14} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className="thread-filter-options min-w-(--radix-dropdown-menu-trigger-width)"
        >
          <DropdownMenuRadioGroup
            value={stage}
            onValueChange={(value) => {
              setStage(value as WorkStage);
              setVisibleLimit(20);
            }}
          >
            {stages.map((entry) => (
              <DropdownMenuRadioItem key={entry.id} value={entry.id}>
                {stageLabel(entry.id)}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {(filter || stage !== "all") && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            setFilter("");
            setStage("all");
            setVisibleLimit(20);
          }}
        >
          Clear filters
        </Button>
      )}
      {query.isError && (
        <ErrorNotice
          error={query.error}
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {query.isPending && <p role="status">Loading work…</p>}
      <div className="conversation-work-items">
        {!!term && titleState.pending > 0 && (
          <p className="conversation-muted px-2 py-2" role="status">
            Searching thread titles… {titleItems.length - titleState.pending} of{" "}
            {titleItems.length} checked.
          </p>
        )}
        {titleState.failed > 0 && (
          <div className="conversation-muted px-2 py-2" role="status">
            <p>
              {titleState.failed}{" "}
              {titleState.failed === 1 ? "title is" : "titles are"} unavailable.
              You can still find threads by ID or file name.{" "}
              {term
                ? "Search results may be incomplete."
                : "Their worker may be stopped."}
            </p>
            <Button
              variant="outline"
              size="sm"
              className="mt-2"
              disabled={titleState.retrying || titleState.pending > 0}
              onClick={() => void titleState.retry()}
            >
              {titleState.retrying ? "Retrying titles…" : "Retry titles"}
            </Button>
          </div>
        )}
        {visible.map((item) => renderThread(item))}
        {matches.length > 0 && (
          <div className="px-2 py-3 text-center">
            <p className="conversation-muted" role="status">
              Showing {visible.length} of {matches.length}{" "}
              {term && titleState.failed > 0 ? "available " : ""}
              {term
                ? matches.length === 1
                  ? "match"
                  : "matches"
                : matches.length === 1
                  ? "thread"
                  : "threads"}
              {term && titleState.pending > 0 ? " so far" : ""}
            </p>
            {visible.length < matches.length && (
              <Button
                variant="outline"
                size="sm"
                className="mt-2 w-full"
                onClick={() => setVisibleLimit((limit) => limit + 20)}
              >
                Show more threads
              </Button>
            )}
          </div>
        )}
        {!query.isPending &&
          !query.isError &&
          !titleState.pending &&
          !visible.length && (
            <p className="conversation-muted">
              {items.length
                ? titleState.failed && term
                  ? "No matches in the available titles. Try an ID or file name, or retry titles."
                  : "No threads match these filters."
                : "Your conversations and their runs will appear here."}
            </p>
          )}
      </div>
    </aside>
  );
  const showThreadsLabel = usesDrawer ? "Switch thread" : "Show thread panel";
  return (
    <div
      className="work-page"
      style={
        {
          "--thread-rail-width": `${clampRailWidth(railWidth)}px`,
        } as CSSProperties
      }
    >
      <h1 className="sr-only">Project work</h1>
      <div
        className={`conversation-workbench ${selected ? "has-conversation" : ""}`}
        data-thread-rail-compact={compactRail}
        data-thread-drawer={usesDrawer}
      >
        {compactRail ? (
          <aside
            className="conversation-thread-rail thread-rail-collapsed"
            aria-label="Thread panel"
          >
            <div className="thread-rail-heading">
              <Button
                ref={showPanelButton}
                variant="outline"
                size="icon-lg"
                aria-label={showThreadsLabel}
                title={usesDrawer ? "Threads" : showThreadsLabel}
                aria-haspopup={usesDrawer ? "dialog" : undefined}
                aria-expanded={usesDrawer && worklist}
                onClick={usesDrawer ? () => setWorklist(true) : togglePanel}
              >
                <MessagesSquare size={16} />
              </Button>
            </div>

            <div className="conversation-work-items">
              {visible.map((item) => renderThread(item, true))}
              {visible.length < matches.length && (
                <Button
                  variant="ghost"
                  size="icon"
                  className={usesDrawer ? "size-11" : undefined}
                  aria-label="Show more threads"
                  title="Show more threads"
                  onClick={() => setVisibleLimit((limit) => limit + 20)}
                >
                  <ChevronDown size={16} />
                </Button>
              )}
            </div>
          </aside>
        ) : (
          <div className="conversation-thread-rail" ref={rail}>
            <div id="thread-sidebar" className="conversation-rail-expanded">
              {renderWorkList(true)}
            </div>
            <div
              className="thread-resizer"
              role="separator"
              aria-label="Resize thread list"
              aria-orientation="vertical"
              aria-valuemin={minRailWidth}
              aria-valuemax={maxRailWidth}
              aria-valuenow={clampRailWidth(railWidth)}
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === "Home" || e.key === "End") {
                  e.preventDefault();
                  setRailWidth(e.key === "Home" ? minRailWidth : maxRailWidth);
                } else if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
                  e.preventDefault();
                  setRailWidth((w) =>
                    clampRailWidth(w + (e.key === "ArrowRight" ? 4 : -4)),
                  );
                }
              }}
              onPointerDown={(e) => {
                e.preventDefault();
                e.currentTarget.setPointerCapture(e.pointerId);
              }}
              onPointerMove={(e) => {
                if (
                  e.currentTarget.hasPointerCapture(e.pointerId) &&
                  rail.current
                )
                  setRailWidth(
                    clampRailWidth(
                      e.clientX - rail.current.getBoundingClientRect().left,
                    ),
                  );
              }}
              onPointerUp={(e) =>
                e.currentTarget.releasePointerCapture(e.pointerId)
              }
            />
          </div>
        )}
        <Sheet open={worklist} onOpenChange={setWorklist}>
          <SheetContent
            side="left"
            className="conversation-thread-drawer w-[320px] max-w-[calc(100vw-24px)] p-4"
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              (showPanelButton.current ?? hidePanelButton.current)?.focus();
            }}
            aria-describedby={undefined}
          >
            <SheetTitle>Threads</SheetTitle>
            {renderWorkList()}
          </SheetContent>
        </Sheet>
        {selected ? (
          <WorkConversation
            key={selected}
            project={project}
            id={selected}
            title={titles.get(selected)}
            owner={titleState.owners.get(selected)}
            message={drafts[selected] || ""}
            setMessage={setThreadDraft}
          />
        ) : (
          <div className="new-thread-surface">
            <div className="new-thread-navigation">New thread</div>
            <NewConversation
              project={project}
              onCreated={(item) => {
                personal.focusComposer.current = item.id;
                select(item.id, startsInWorkspace(item.kind));
              }}
            />
          </div>
        )}
      </div>
    </div>
  );
}

function NewConversation({
  project,
  onCreated,
}: {
  project: Project;
  onCreated: (item: WorkItem) => void;
}) {
  const [provider, setProvider] = useState("");
  const [model, setModel] = useState("");
  const [reasoning, setReasoning] = useState("");
  const [workspace, setWorkspace] = useState<string>();
  const [defaultModelConfirmed, setDefaultModelConfirmed] = useState(false);
  const [kind, setKind] = useState<WorkProfile>("software");
  const [outputs, setOutputs] = useState("");
  const workers = useQuery(workspaceOptions(project.id));
  const availableWorkers =
    workers.data?.filter(
      (w) => w.agent_kind !== "hermes" && !w.retained_preview,
    ) || [];
  const selectedWorkspace =
    workspace ??
    (availableWorkers.find((w) => w.ready)?.id ||
      availableWorkers[0]?.id ||
      "");
  const automaticWorker =
    !selectedWorkspace && (workspace === "" || workers.isSuccess);
  const key = useRequestKey();
  const client = useQueryClient();
  const outputPaths = outputs
    .split(/\r?\n/)
    .map((path) => path.trim())
    .filter(Boolean);
  const tooManyOutputs = outputPaths.length > 32;
  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        project_id: project.id,
        workspace_id: selectedWorkspace,
        ...(provider ? { provider } : {}),
        ...(provider && model ? { model } : {}),
        ...(reasoning ? { reasoning_effort: reasoning } : {}),
        empty_thread: true as const,
        kind,
        artifact_paths: outputPaths,
      };
      return workApi.create(payload, key(payload));
    },
    onSuccess: async (item) => {
      await client.invalidateQueries({ queryKey: workKeys.list(project.id) });
      await client.invalidateQueries({
        queryKey: ["project", project.id, "workspaces"],
      });
      onCreated(item);
    },
  });
  const canSend =
    !mutation.isPending &&
    (automaticWorker
      ? defaultModelConfirmed
      : !!(selectedWorkspace && provider && model)) &&
    !tooManyOutputs;
  return (
    <section className="conversation-new" aria-label="New work conversation">
      <div className="conversation-intro">
        <div className="conversation-intro-heading">
          <MessageSquare size={26} aria-hidden="true" />
          <h2>New thread</h2>
        </div>
      </div>
      <Form
        onSubmit={() => {
          if (canSend) mutation.mutate();
        }}
      >
        {automaticWorker ? (
          <NativeSelect
            aria-label="Model"
            value={defaultModelConfirmed ? "worker-default" : ""}
            onChange={(e) => {
              setWorkspace("");
              setDefaultModelConfirmed(e.target.value === "worker-default");
            }}
            disabled={mutation.isPending}
          >
            <option value="" disabled>
              Choose a model
            </option>
            <option value="worker-default">
              Use the new worker’s default model
            </option>
          </NativeSelect>
        ) : (
          <AgentSelection
            workspace={selectedWorkspace}
            provider={provider}
            model={model}
            reasoning={reasoning}
            onReasoning={(effort, nextProvider, nextModel) => {
              setWorkspace(selectedWorkspace);
              setReasoning(effort);
              setProvider(nextProvider);
              setModel(nextModel);
            }}
            disabled={mutation.isPending}
            requireModel
          />
        )}
        <div className="conversation-new-options">
          <label>
            Worker
            <NativeSelect
              aria-label="Worker"
              value={selectedWorkspace}
              onChange={(e) => {
                setWorkspace(e.target.value);
                setDefaultModelConfirmed(false);
                setProvider("");
                setModel("");
                setReasoning("");
              }}
              disabled={mutation.isPending}
            >
              <option value="">Automatic worker</option>
              {availableWorkers.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name || w.id}
                </option>
              ))}
            </NativeSelect>
          </label>
          <label>
            Work profile
            <NativeSelect
              aria-label="Work profile"
              value={kind}
              onChange={(e) => setKind(e.target.value as WorkProfile)}
              disabled={mutation.isPending}
            >
              {workProfiles.map(({ kind, label }) => (
                <option key={kind} value={kind}>
                  {label}
                </option>
              ))}
            </NativeSelect>
          </label>
        </div>
        <details className="conversation-output-settings">
          <summary>Keep specific output files</summary>
          <label>
            Retained output paths
            <Textarea
              rows={3}
              aria-invalid={tooManyOutputs || undefined}
              aria-describedby="new-work-outputs-help"
              value={outputs}
              onChange={(e) => setOutputs(e.target.value)}
              placeholder={"outputs/report.md\noutputs/results.json"}
              disabled={mutation.isPending}
            />
          </label>
          <p id="new-work-outputs-help">
            One worker file per line, up to 32. Ask the agent to create these
            files; each run saves a separate copy for review.
          </p>
          {tooManyOutputs && (
            <p className="text-sm text-destructive" role="alert">
              You selected {outputPaths.length} output files. Keep at most 32
              paths before creating the thread.
            </p>
          )}
        </details>
        {workers.isError && (
          <ErrorNotice
            error={workers.error}
            onRetry={() => void workers.refetch()}
            retrying={workers.isFetching}
          />
        )}
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="conversation-compose-actions">
          <Button disabled={!canSend}>
            {mutation.isPending ? "Creating…" : "Create thread"}
          </Button>
        </div>
      </Form>
    </section>
  );
}
