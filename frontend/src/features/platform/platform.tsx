import {
  Boxes,
  Monitor,
  Network,
  Files,
  ShieldCheck,
  TerminalSquare,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";

const guides = [
  {
    icon: TerminalSquare,
    title: "Start with a conversation",
    body: "Open a project and choose Workspace → New thread. Describe the result you need, select a worker, and choose Start work. Follow the agent’s messages and activity in the conversation.",
  },
  {
    icon: ShieldCheck,
    title: "Review before accepting",
    body: "Use Issue board to track progress and Reviews to inspect completed work. Check the preview, code and reports. Choose Approve when the result meets your requirements, or Request changes with specific feedback.",
  },
  {
    icon: Boxes,
    title: "Choose a project and worker",
    body: "Open a project, then use Workers to create an isolated workspace for T3 and Codex. Choose a connection profile to control network access.",
  },
  {
    icon: Monitor,
    title: "Connect T3 desktop",
    body: "Use the desktop button on a worker row, then Open T3 desktop in the pairing dialog. You can also paste its one-use link into desktop Settings → Environments → Add environment. Follow the connection requirements shown in the pairing dialog.",
  },
  {
    icon: Network,
    title: "Choose what it can reach",
    body: "Open Platform → Network profiles in the sidebar. Assign one through worker Settings. Allowlist entries are exact host:port pairs. Fully open uses direct networking and DNS; stop the worker before switching that mode.",
  },
  {
    icon: Files,
    title: "Share a project file",
    body: "Open Workers → Project actions → Copy a file. Transfers stay within the project and preserve existing destination files. Private agent history is not copied.",
  },
  {
    icon: TerminalSquare,
    title: "Keep work available",
    body: "Worker Settings controls automatic idle shutdown. Opening it resets the timer; active runs and tracked background jobs prevent shutdown. Stopping preserves files and conversations.",
  },
];
export function GettingStarted() {
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Getting started</h1>
        </div>
        <Button asChild>
          <Link to="/">Open projects</Link>
        </Button>
      </div>
      <div className="grid min-w-0 gap-6 [overflow-wrap:anywhere] lg:grid-cols-[minmax(0,1fr)_300px]">
        <div className="min-w-0 rounded-xl border bg-background">
          {guides.map((item) => (
            <section
              key={item.title}
              className="flex min-w-0 flex-col items-start gap-3 border-b p-4 last:border-0 sm:flex-row"
            >
              <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/40 text-muted-foreground">
                <item.icon className="size-4" />
              </span>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold">{item.title}</h2>
                <p className="mt-2 max-w-prose text-sm leading-6 text-muted-foreground">
                  {item.body}
                </p>
              </div>
            </section>
          ))}
        </div>
        <aside className="min-w-0 space-y-5">
          <section className="rounded-xl border bg-background p-5">
            <h2 className="text-sm font-semibold">Operator troubleshooting</h2>
            <p className="mt-3 text-xs leading-6 text-muted-foreground">
              Use Workers → Subscription to save your Codex ChatGPT login and
              load it into a worker. If it expires, import a current login. For
              legacy Vault-managed credentials, ask an administrator to follow
              docs/subscription-agents.md.
            </p>
            <p className="mt-3 text-xs leading-6 text-muted-foreground">
              Expired logins need refreshing through the native provider CLI.
              There is no automatic API-billing fallback.
            </p>
          </section>
          <section className="rounded-xl border bg-background p-5">
            <ShieldCheck className="mb-3 size-5 text-muted-foreground" />
            <h2 className="text-sm font-semibold">Workspace storage</h2>
            <p className="mt-3 text-xs leading-6 text-muted-foreground">
              Private workspaces keep project files and agent state separate. In
              shared-workspace mode, all projects use the same files,
              conversations and connection settings.
            </p>
          </section>
        </aside>
      </div>
    </>
  );
}
