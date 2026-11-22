import { useState } from "react";
import { Plus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import type { SetupInventory, SetupOperation } from "./setup-api";

export function SetupSkills({
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
  const [source, setSource] = useState("");
  const [skill, setSkill] = useState("");
  const [addingSkill, setAddingSkill] = useState(false);
  const job = inventory.job;
  const discovery = job?.state === "discovered" ? job.discovery : undefined;
  const skillChoice = discovery?.names.includes(skill)
    ? skill
    : (discovery?.names[0] ?? "");
  return (
    <div className="min-w-0 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-mono text-xs text-muted-foreground">
          skills-lock.json
        </span>
        <div className="flex flex-wrap gap-2">
          {inventory.skill_lock &&
            inventory.skills.some((entry) => entry.state === "missing") && (
              <Button
                size="sm"
                variant="outline"
                disabled={locked || isBusy || !!inventory.skill_issue}
                onClick={() => change({ operation: "skills-restore" })}
              >
                Restore skills
              </Button>
            )}
          <Button
            size="sm"
            variant="outline"
            disabled={locked || isBusy || !!inventory.skill_issue}
            onClick={() => setAddingSkill(true)}
          >
            <Plus aria-hidden="true" /> Add skill
          </Button>
        </div>
      </div>
      {(addingSkill || discovery) && !locked && !inventory.skill_issue && (
        <div className="space-y-3 rounded-md border border-primary/30 bg-card p-4">
          <h2 className="text-sm font-semibold">Add a skill from GitHub</h2>
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              setSkill("");
              change({
                operation: "skill-discover",
                source: source.trim(),
              });
            }}
          >
            <label className="grid min-w-0 flex-1 gap-1.5 text-sm">
              GitHub repository
              <Input
                required
                autoFocus={!discovery}
                maxLength={180}
                placeholder="owner/repository"
                value={source}
                onChange={(event) => setSource(event.target.value)}
              />
            </label>
            <Button type="submit" variant="outline" disabled={isBusy}>
              Find skills
            </Button>
          </form>
          {discovery &&
            (discovery.names.length ? (
              <form
                className="flex flex-wrap items-end gap-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  change({
                    operation: "skill-add",
                    source: discovery.source,
                    commit: discovery.commit,
                    name: skillChoice,
                  });
                }}
              >
                <label className="grid min-w-0 flex-1 gap-1.5 text-sm">
                  Skill in {discovery.source}
                  <NativeSelect
                    value={skillChoice}
                    onChange={(event) => setSkill(event.target.value)}
                  >
                    {discovery.names.map((name) => (
                      <option key={name}>{name}</option>
                    ))}
                  </NativeSelect>
                </label>
                <Button type="submit" disabled={isBusy}>
                  Review skill
                </Button>
              </form>
            ) : (
              <p className="text-sm text-muted-foreground">
                No skills found in {discovery.source}.
              </p>
            ))}
          <Button
            size="sm"
            type="button"
            variant="ghost"
            onClick={() => {
              setAddingSkill(false);
              setSource("");
              if (job?.state === "discovered")
                change({ operation: "discard", plan_id: job.id });
            }}
          >
            Cancel
          </Button>
        </div>
      )}
      {inventory.skill_issue ? (
        <p className="rounded-md border bg-muted/30 p-3 text-sm">
          Setup couldn’t read the existing skills. Check skills-lock.json and
          .agents/skills in the checkout.
        </p>
      ) : inventory.skills.length ? (
        <ul className="divide-y border-y">
          {inventory.skills.map((entry) => (
            <li
              key={entry.name}
              className="flex flex-wrap items-center justify-between gap-2 py-3 text-sm"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="break-all font-medium">{entry.name}</span>
                  <Badge variant="outline">
                    {entry.state === "local"
                      ? "Local"
                      : entry.state === "installed"
                        ? "Installed"
                        : "Missing"}
                  </Badge>
                </div>
                {entry.source && (
                  <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                    {entry.source}
                    {entry.commit ? ` @ ${entry.commit.slice(0, 10)}` : ""}
                  </p>
                )}
              </div>
              {entry.state !== "local" && (
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Remove ${entry.name}`}
                  disabled={locked || isBusy}
                  onClick={() =>
                    change({
                      operation: "skill-remove",
                      name: entry.name,
                    })
                  }
                >
                  Remove
                </Button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          Add a project skill for {inventory.skills_target} from a GitHub
          repository.
        </p>
      )}
    </div>
  );
}
