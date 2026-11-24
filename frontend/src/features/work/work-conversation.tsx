import { usePersonalWork, useLayoutPreference } from "./personal-work";
import { AgentSelection } from "./agent-selection";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { conversationTitle } from "./conversation-title";
import { useThreadTitles } from "./thread-titles";
import { composerKeyDown } from "./composer-keydown";
import {
  useState,
  useEffect,
  type CSSProperties,
  useCallback,
  useRef,
  type SetStateAction,
  type Dispatch,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import {
  Plus,
  Search,
  MessageSquare,
  Bot,
  ArrowUp,
  PanelLeftOpen,
  ChevronDown,
} from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { ErrorNotice, Form, Modal, useRequestKey } from "@/shared/components";
import { date, label } from "@/shared/format";
import type { Project } from "@/shared/types";
import { workspaceOptions } from "@/features/workspaces/api";
import { ProjectCompanion } from "@/features/project-assistant/companion";
import { WorkConversation } from "./work-thread";
import {
  workApi,
  workKeys,
  workOptions,
  type WorkCreate,
  type WorkItem,
} from "./api";
import { stages, workStage, workStatus, type WorkStage } from "./stages";

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
  const rail = useRef<HTMLDivElement>(null);
  useEffect(() => {
    personal.setLastThread(selected);
  }, [selected]);
  const query = useQuery(workOptions(project.id));
  const [filter, setFilter] = useState("");
  const [visibleLimit, setVisibleLimit] = useState(20);
  const [stage, setStage] = useState<WorkStage>("all");
  const [worklist, setWorklist] = useState(false);
  const threadSwitch = useRef<HTMLButtonElement>(null);
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
  const [assistant, setAssistant] = useState(false);
  const [startError, setStartError] = useState<unknown>();
  const select = (id: string, openWorkspace = false) => {
    setStartError(undefined);
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
  const renderWorkList = () => (
    <aside className="conversation-worklist" aria-label="Threads">
      <Button className="w-full" onClick={() => select("")}>
        <Plus size={14} /> New thread
      </Button>
      <div className="conversation-search">
        <Search size={14} />
        <Input
          className="h-8 pl-8 text-xs"
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
        {visible.map((item) => (
          <button
            key={item.id}
            className={`conversation-work-item ${selected === item.id ? "is-selected" : ""}`}
            onClick={() => select(item.id)}
            aria-pressed={selected === item.id}
            aria-description={
              drafts[item.id]?.trim() ? "Unsent draft" : undefined
            }
            title={titles.get(item.id)}
          >
            <span className="conversation-work-name">
              <span className="line-clamp-2">{titles.get(item.id)}</span>
              {titles.get(item.id) === "Untitled conversation" && (
                <small>{item.id}</small>
              )}
            </span>
            <span className="conversation-work-meta">
              <span>{titleState.owners.get(item.id) || "Unassigned"}</span>
              {workStatus(item.state) !== "accepted" && (
                <span
                  className="thread-state"
                  data-stage={workStage(item.state)}
                  data-state={workStatus(item.state)}
                >
                  {label(workStatus(item.state))}
                </span>
              )}
              {drafts[item.id]?.trim() && (
                <span className="thread-draft">Draft</span>
              )}
              <time title={date(item.created_at)}>
                {new Date(item.created_at).toLocaleDateString(undefined, {
                  month: "short",
                  day: "numeric",
                })}
              </time>
            </span>
          </button>
        ))}
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
      <div className="conversation-rail-footer">
        <Button variant="ghost" size="sm" onClick={() => setAssistant(true)}>
          <Bot size={14} /> Project assistant
        </Button>
      </div>
    </aside>
  );
  const mobileNavigation = (
    <Button
      ref={threadSwitch}
      className="conversation-drawer-toggle"
      variant="ghost"
      size="icon-sm"
      aria-label="Switch thread"
      title="Threads"
      aria-expanded={worklist}
      onClick={() => setWorklist(true)}
    >
      <PanelLeftOpen size={18} />
    </Button>
  );
  return (
    <div
      className="work-page"
      style={
        {
          "--thread-rail-width": `${Math.max(240, Math.min(280, railWidth))}px`,
        } as CSSProperties
      }
    >
      <h1 className="sr-only">Project work</h1>
      <div
        className={`conversation-workbench ${selected ? "has-conversation" : ""}`}
      >
        <div className="conversation-thread-rail" ref={rail}>
          <div id="thread-sidebar" className="conversation-rail-expanded">
            {renderWorkList()}
          </div>
          <div
            className="thread-resizer"
            role="separator"
            aria-label="Resize thread list"
            aria-orientation="vertical"
            aria-valuemin={240}
            aria-valuemax={280}
            aria-valuenow={railWidth}
            tabIndex={0}
            onKeyDown={(e) => {
              if (["ArrowLeft", "ArrowRight"].includes(e.key)) {
                e.preventDefault();
                setRailWidth((w) =>
                  Math.max(
                    240,
                    Math.min(280, w + (e.key === "ArrowRight" ? 4 : -4)),
                  ),
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
                  Math.max(
                    240,
                    Math.min(
                      280,
                      e.clientX - rail.current.getBoundingClientRect().left,
                    ),
                  ),
                );
            }}
            onPointerUp={(e) =>
              e.currentTarget.releasePointerCapture(e.pointerId)
            }
          />
        </div>
        <Sheet open={worklist} onOpenChange={setWorklist}>
          <SheetContent
            side="left"
            className="conversation-thread-drawer w-[320px] max-w-[calc(100vw-24px)] p-4"
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              threadSwitch.current?.focus();
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
            startError={startError}
            title={titles.get(selected)}
            owner={titleState.owners.get(selected)}
            navigation={mobileNavigation}
            message={drafts[selected] || ""}
            setMessage={setThreadDraft}
          />
        ) : (
          <div className="new-thread-surface">
            <div className="new-thread-navigation">
              {mobileNavigation}
              <span>New thread</span>
            </div>
            <NewConversation
              project={project}
              instruction={drafts[""] || ""}
              setInstruction={setThreadDraft}
              onStarted={(item, error) => {
                setThreadDraft("");
                select(item.id, item.kind === "data-science");
                setStartError(error);
              }}
            />
          </div>
        )}
      </div>
      {assistant && (
        <Modal
          title={`Project assistant in ${project.name}`}
          onClose={() => setAssistant(false)}
        >
          <ProjectCompanion project={project} />
        </Modal>
      )}
    </div>
  );
}

function NewConversation({
  project,
  onStarted,
  instruction,
  setInstruction,
}: {
  instruction: string;
  setInstruction: Dispatch<SetStateAction<string>>;
  project: Project;
  onStarted: (item: WorkItem, error?: unknown) => void;
}) {
  const [provider, setProvider] = useState("");
  const [model, setModel] = useState("");
  const [reasoning, setReasoning] = useState("");
  const [workspace, setWorkspace] = useState("");
  const [kind, setKind] = useState<WorkCreate["kind"] | "">("");
  const [outputs, setOutputs] = useState("");
  const workers = useQuery(workspaceOptions(project.id));
  const availableWorkers =
    workers.data?.filter(
      (w) => w.agent_kind !== "hermes" && !w.retained_preview,
    ) || [];
  const selectedWorkspace =
    workspace ||
    availableWorkers.find((w) => w.ready)?.id ||
    availableWorkers[0]?.id ||
    "";
  const key = useRequestKey();
  const client = useQueryClient();
  const instructionTooLong =
    new TextEncoder().encode(instruction).length > 65536;
  const outputPaths = outputs
    .split(/\r?\n/)
    .map((path) => path.trim())
    .filter(Boolean);
  const tooManyOutputs = outputPaths.length > 32;
  const mutation = useMutation({
    mutationFn: () => {
      if (!kind) throw new Error("Choose a work type.");
      const payload = {
        project_id: project.id,
        workspace_id: selectedWorkspace,
        ...(provider ? { provider } : {}),
        ...(provider && model ? { model } : {}),
        ...(reasoning ? { reasoning_effort: reasoning } : {}),
        instruction,
        kind,
        artifact_paths: outputPaths,
      };
      return workApi.begin(payload, key(payload));
    },
    onSuccess: async ({ item, error }) => {
      await client.invalidateQueries({ queryKey: workKeys.list(project.id) });
      await client.invalidateQueries({
        queryKey: ["project", project.id, "workspaces"],
      });
      onStarted(item, error);
    },
  });
  const canSend =
    !mutation.isPending &&
    !!kind &&
    !!instruction.trim() &&
    !instructionTooLong &&
    !tooManyOutputs;
  return (
    <section className="conversation-new" aria-label="New work conversation">
      <div className="conversation-intro">
        <div className="conversation-intro-heading">
          <MessageSquare size={26} aria-hidden="true" />
          <h2>What are we working on?</h2>
        </div>
      </div>
      <Form
        onSubmit={() => {
          if (canSend) mutation.mutate();
        }}
      >
        <label htmlFor="new-work-message" className="sr-only">
          Message to your agent
        </label>
        <Textarea
          id="new-work-message"
          aria-invalid={instructionTooLong || undefined}
          aria-describedby={
            instructionTooLong ? "new-work-message-error" : undefined
          }
          rows={7}
          autoFocus
          placeholder="What would you like the agent to do? Include how you'll know it's done."
          value={instruction}
          onChange={(e) => setInstruction(e.target.value)}
          onKeyDown={(e) => composerKeyDown(e, canSend)}
          required
          disabled={mutation.isPending}
        />
        {instructionTooLong && (
          <p
            id="new-work-message-error"
            className="text-sm text-destructive"
            role="alert"
          >
            This message exceeds 64 KiB. Shorten it or save supporting material
            in a worker file, then refer to that file in your instructions.
          </p>
        )}
        <AgentSelection
          workspace={selectedWorkspace}
          provider={provider}
          model={model}
          reasoning={reasoning}
          onReasoning={(effort, nextProvider, nextModel) => {
            setReasoning(effort);
            setProvider(nextProvider);
            setModel(nextModel);
          }}
          disabled={mutation.isPending}
        />
        <div className="conversation-new-options">
          <label>
            Worker
            <NativeSelect
              aria-label="Worker"
              value={selectedWorkspace}
              onChange={(e) => {
                setWorkspace(e.target.value);
                setProvider("");
                setModel("");
                setReasoning("");
              }}
              disabled={mutation.isPending}
            >
              <option value="">Automatic worker</option>
              {workers.data
                ?.filter(
                  (w) => w.agent_kind !== "hermes" && !w.retained_preview,
                )
                .map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.name || w.id}
                  </option>
                ))}
            </NativeSelect>
          </label>
          <label>
            Work type
            <NativeSelect
              aria-label="Work type"
              value={kind}
              onChange={(e) =>
                setKind(e.target.value as WorkCreate["kind"] | "")
              }
              disabled={mutation.isPending}
            >
              <option value="" disabled>
                Choose a work type
              </option>
              <option value="software">Software</option>
              <option value="data-science">Data science</option>
              <option value="research">Research</option>
              <option value="digest">Digest</option>
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
              paths before starting work.
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
            <ArrowUp size={15} />
            {mutation.isPending ? "Starting…" : "Start work"}
          </Button>
        </div>
      </Form>
    </section>
  );
}
