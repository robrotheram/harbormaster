import { useNavigate, useSearch } from "@tanstack/react-router";
import { Tabs } from "radix-ui";
import { Bot, FolderGit2 } from "lucide-react";
import { AgentSubscriptions } from "./subscriptions";
import { RepositoryConnections } from "@/features/delivery/connections";
import type { Project } from "@/shared/types";

const tabClassName =
  "inline-flex min-w-0 items-center justify-center gap-2 border-b-2 border-transparent px-3 py-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset data-[state=active]:border-primary data-[state=active]:text-foreground";

export function Connections({
  projects,
  repositories = false,
}: {
  projects: Project[];
  repositories?: boolean;
}) {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { tab?: string };
  const selected =
    repositories || search.tab === "repositories" ? "repositories" : "agents";
  return (
    <div className="space-y-6">
      <div className="page-title">
        <div>
          <h1>Connections</h1>
        </div>
      </div>
      <Tabs.Root
        value={selected}
        onValueChange={(tab) =>
          void navigate({
            to: "/connections",
            search: { tab: tab === "repositories" ? "repositories" : "agents" },
          })
        }
      >
        <Tabs.List aria-label="Connection categories" className="flex border-b">
          <Tabs.Trigger value="agents" className={tabClassName}>
            <Bot aria-hidden="true" className="size-4 shrink-0" />
            Agent subscriptions
          </Tabs.Trigger>
          <Tabs.Trigger value="repositories" className={tabClassName}>
            <FolderGit2 aria-hidden="true" className="size-4 shrink-0" />
            Repositories
          </Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content
          value="agents"
          className="pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <AgentSubscriptions />
        </Tabs.Content>
        <Tabs.Content
          value="repositories"
          className="space-y-6 pt-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <p className="form-note">
            Repository connections are shared across the platform.
          </p>
          <RepositoryConnections projects={projects} embedded />
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
