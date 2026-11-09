import { toolProfileOptions } from "./api";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Pencil } from "lucide-react";
import { keys } from "@/shared/query-keys";

import { toolProfilesApi } from "@/features/tool-profiles/api";
import { ErrorNotice, Form, Modal } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import type { Profile } from "@/shared/types";
const tools = [
  {
    id: "workspace-files",
    name: "Workspace files",
    description: "Read, write and list files in the workspace.",
  },
  {
    id: "project-git",
    name: "Project Git",
    description: "Read Git status, diffs and recent commits.",
  },
];
export function ToolProfiles() {
  const query = useQuery(toolProfileOptions());
  const [edit, setEdit] = useState<Profile | null>();
  return (
    <>
      <div className="page-title">
        <div>
          <div className="mb-2 flex items-center gap-2">
            <h1>Tool profiles</h1>
            <Badge variant="outline">Global</Badge>
          </div>
          <p>Choose reusable sets of bundled MCP tools for your workspaces.</p>
        </div>
        <Button onClick={() => setEdit(null)}>
          <Plus />
          Create profile
        </Button>
      </div>
      <p className="mb-5 max-w-3xl text-sm leading-6 text-muted-foreground">
        Changes apply on the next workspace stop/start. These profiles configure
        MCP tools for T3, Codex and Claude; built-in agent tools and sandbox
        permissions are separate. Hermes uses its own toolsets.
      </p>
      {query.isError && <ErrorNotice error={query.error} />}
      {query.isPending ? (
        <p>Loading profiles…</p>
      ) : (
        <div className="overflow-x-auto rounded-xl border bg-background">
          <table
            className="w-full text-left text-sm"
            aria-label="Global tool profiles"
          >
            <thead className="border-b bg-muted/40">
              <tr>
                <th className="p-4">Profile</th>
                <th className="p-4">Tools</th>
                <th className="p-4">Version</th>
                <th className="p-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {query.data?.map((p) => (
                <tr key={p.id} className="border-b last:border-0">
                  <td className="p-4">
                    <h2 className="font-semibold">{p.title}</h2>
                    <p className="mt-1 max-w-md text-xs leading-5 text-muted-foreground">
                      {p.description}
                    </p>
                  </td>
                  <td className="p-4">
                    <div className="flex flex-wrap gap-1">
                      {p.mcps.length ? (
                        p.mcps.map((name) => (
                          <Badge key={name} variant="secondary">
                            {tools.find((t) => t.id === name)?.name || name}
                          </Badge>
                        ))
                      ) : (
                        <span className="text-muted-foreground">
                          No MCP tools
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="p-4">{p.revision}</td>
                  <td className="p-4 text-right">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setEdit(p)}
                      aria-label={`Edit ${p.title}`}
                    >
                      <Pencil />
                      Edit
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {edit !== undefined && (
        <ToolProfileForm profile={edit} onClose={() => setEdit(undefined)} />
      )}
    </>
  );
}
function ToolProfileForm({
  profile,
  onClose,
}: {
  profile: Profile | null;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [title, setTitle] = useState(profile?.title || "");
  const [description, setDescription] = useState(profile?.description || "");
  const [mcps, setMCPs] = useState<string[]>(
    profile?.mcps || tools.map((t) => t.id),
  );
  const save = useMutation({
    mutationFn: () =>
      toolProfilesApi.save(profile, { title, description, mcps }),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: keys.profiles });
      onClose();
    },
  });
  return (
    <Modal
      title={profile ? "Edit tool profile" : "Create tool profile"}
      onClose={onClose}
    >
      <Form onSubmit={() => save.mutate()}>
        <label>
          Name
          <Input
            autoFocus
            required
            maxLength={80}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
        </label>
        <label>
          Description
          <Textarea
            maxLength={500}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <fieldset className="space-y-3">
          <legend className="mb-2 text-sm font-medium">MCP tools</legend>
          {tools.map((t) => (
            <label
              key={t.id}
              className="flex items-start gap-3 rounded-lg border p-3"
            >
              <input
                type="checkbox"
                checked={mcps.includes(t.id)}
                onChange={(e) =>
                  setMCPs((values) =>
                    e.target.checked
                      ? [...values, t.id]
                      : values.filter((v) => v !== t.id),
                  )
                }
              />
              <span>
                <span className="block text-sm font-medium">{t.name}</span>
                <span className="block text-xs font-normal text-muted-foreground">
                  {t.description}
                </span>
              </span>
            </label>
          ))}
        </fieldset>
        {save.isError && <ErrorNotice error={save.error} />}
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? "Saving…" : "Save profile"}
        </Button>
      </Form>
    </Modal>
  );
}
