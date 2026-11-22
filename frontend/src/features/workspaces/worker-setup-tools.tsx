import { useState } from "react";
import { Plus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import type { SetupInventory, SetupOperation } from "./setup-api";

const toolLabels: Record<string, string> = {
  bun: "Bun",
  go: "Go",
  node: "Node.js",
  pnpm: "pnpm",
  python: "Python",
  rust: "Rust",
  uv: "uv",
};

export function SetupTools({
  inventory,
  locked,
  isBusy,
  change,
}: {
  inventory: SetupInventory;
  locked: boolean;
  isBusy: boolean;
  change: (operation: SetupOperation) => void;
}) {
  const [tool, setTool] = useState<{ name: string; version: string }>();
  return (
    <div className="min-w-0 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-mono text-xs text-muted-foreground">
          {inventory.tool_config} + {inventory.tool_lock}
        </span>
        <div className="flex flex-wrap gap-2">
          {inventory.tools_need_sync && (
            <Button
              size="sm"
              variant={tool ? "outline" : "default"}
              disabled={!!tool || locked || isBusy || !!inventory.tool_issue}
              onClick={() => change({ operation: "tools-restore" })}
            >
              Sync tools
            </Button>
          )}
          <Button
            size="sm"
            variant={inventory.tools_need_sync || tool ? "outline" : "default"}
            disabled={
              !!tool ||
              locked ||
              isBusy ||
              !!inventory.tool_issue ||
              inventory.tools.length === inventory.catalog.length
            }
            onClick={() =>
              setTool({
                name: "",
                version: "latest",
              })
            }
          >
            <Plus aria-hidden="true" /> Add tool
          </Button>
        </div>
      </div>
      {tool && !locked && !inventory.tool_issue && (
        <form
          className="space-y-3 rounded-md border border-primary/30 bg-card p-4"
          onSubmit={(event) => {
            event.preventDefault();
            setTool(undefined);
            change({
              operation: "tool-set",
              name: tool.name,
              version: tool.version.trim(),
            });
          }}
        >
          <h2 className="text-sm font-semibold">
            {inventory.tools.some((entry) => entry.name === tool.name)
              ? "Change version"
              : "Add a tool"}
          </h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="grid gap-1.5 text-sm">
              Tool
              <NativeSelect
                autoFocus={!tool.name}
                required
                value={tool.name}
                onChange={(event) =>
                  setTool({ ...tool, name: event.target.value })
                }
              >
                <option value="">Choose a tool</option>
                {inventory.catalog
                  .filter(
                    (name) =>
                      name === tool.name ||
                      !inventory.tools.some((entry) => entry.name === name),
                  )
                  .map((name) => (
                    <option key={name} value={name}>
                      {toolLabels[name] ?? name}
                    </option>
                  ))}
              </NativeSelect>
            </label>
            <label className="grid gap-1.5 text-sm">
              Version
              <Input
                autoFocus={!!tool.name}
                required
                maxLength={48}
                placeholder="latest or 24.11.1"
                value={tool.version}
                onChange={(event) =>
                  setTool({ ...tool, version: event.target.value })
                }
              />
            </label>
          </div>
          <p className="text-xs text-muted-foreground">
            Setup pins the exact version shown in the review.
          </p>
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setTool(undefined)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isBusy}>
              Review change
            </Button>
          </div>
        </form>
      )}
      {inventory.tools_need_sync &&
        !inventory.tool_issue &&
        inventory.tools.some((entry) => entry.installed) && (
          <p className="text-sm text-muted-foreground">
            The checkout changed since the last sync.
          </p>
        )}
      {inventory.tool_issue ? (
        <p className="rounded-md border bg-muted/30 p-3 text-sm">
          This checkout uses mise configuration that Setup can’t manage. Use a
          terminal to work with this configuration.
        </p>
      ) : inventory.tools.length ? (
        <div className="divide-y rounded-md border">
          <div
            aria-hidden="true"
            className="hidden gap-3 bg-muted/30 px-4 py-2 text-xs text-muted-foreground md:grid md:grid-cols-[minmax(8rem,1.3fr)_minmax(5rem,0.75fr)_minmax(6rem,1fr)_auto]"
          >
            <span>Tool</span>
            <span>Requested</span>
            <span>Status</span>
            <span className="w-[10rem] text-right">Actions</span>
          </div>
          {inventory.tools.map((entry) => (
            <div
              key={entry.name}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-3 text-sm md:grid md:grid-cols-[minmax(8rem,1.3fr)_minmax(5rem,0.75fr)_minmax(6rem,1fr)_auto] md:px-4"
            >
              <span className="font-medium">
                {toolLabels[entry.name] ?? entry.name}
              </span>
              <span className="text-xs text-muted-foreground">
                <span className="md:sr-only">Requested </span>
                <code>{entry.requested}</code>
              </span>
              <div>
                <Badge variant={entry.installed ? "secondary" : "outline"}>
                  {entry.installed
                    ? `Active ${entry.installed}`
                    : "Needs setup"}
                </Badge>
              </div>
              <div className="flex min-w-full gap-1.5 md:min-w-0 md:w-[10rem] md:justify-end">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Change ${toolLabels[entry.name] ?? entry.name} version`}
                  disabled={locked || isBusy}
                  onClick={() =>
                    setTool({
                      name: entry.name,
                      version: entry.requested,
                    })
                  }
                >
                  Change version
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Remove ${toolLabels[entry.name] ?? entry.name}`}
                  disabled={locked || isBusy}
                  onClick={() =>
                    change({
                      operation: "tool-remove",
                      name: entry.name,
                    })
                  }
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          Add a runtime or CLI to create a mise configuration for this
          workspace.
        </p>
      )}
    </div>
  );
}
