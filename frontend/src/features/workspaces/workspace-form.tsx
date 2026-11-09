import { workspacesApi } from "./api";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { ErrorNotice, Form, Modal, useRequestKey } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import { NativeSelect } from "@/components/ui/native-select";
import { useConnectionProfiles } from "@/features/connection-profiles/api";
import type { Project, Workspace } from "@/shared/types";
export function WorkspaceForm({
  project,
  parent,
  onClose,
  onCreated,
}: {
  project: Project;
  parent?: Workspace;
  onClose: () => void;
  onCreated: () => void;
}) {
  const globalConnections = useConnectionProfiles();
  const [globalConnectionID, setGlobalConnectionID] = useState(
    parent?.connection_profile_id || "",
  );
  const [name, setName] = useState(
    parent ? (parent.name || "Project") + " branch" : "",
  );
  const [agent, setAgent] = useState<string>(parent?.agent_kind || "t3");
  const [branch, setBranch] = useState("");
  const key = useRequestKey();
  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        project_id: project.id,
        ...(globalConnectionID
          ? { connection_profile_id: globalConnectionID }
          : {}),
        name: name.trim(),
        agent_kind: agent,
        ...(parent ? { parent_id: parent.id, branch: branch.trim() } : {}),
      };
      return workspacesApi.create(payload, key(payload));
    },
    onSuccess: onCreated,
  });
  return (
    <Modal
      title={parent ? "Branch workspace" : "Create workspace"}
      onClose={onClose}
    >
      <Form onSubmit={() => mutation.mutate()}>
        <div className="form-project">
          Project <strong>{project.name}</strong>
        </div>
        {parent && (
          <p className="form-note">
            Copy the current project files from{" "}
            {parent.name || "the parent workspace"} into separate storage. Its
            conversations and agent home are not copied. The parent must be
            running and idle.
          </p>
        )}
        <label>
          Workspace name
          <Input
            autoFocus
            required
            maxLength={80}
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Gallery development"
          />
        </label>
        <label>
          Agent
          <NativeSelect
            aria-label="Agent"
            value={agent}
            disabled={!!parent}
            onChange={(event) => setAgent(event.target.value)}
          >
            <option value="t3">T3 with Codex</option>
            <option value="codex">Codex task worker</option>
            <option value="hermes" disabled>
              Hermes — start from Hermes setup
            </option>
          </NativeSelect>
          <Link
            to="/projects/$projectId/$section"
            params={{ projectId: project.id, section: "hermes" }}
            className="text-xs font-normal underline underline-offset-4"
          >
            Configure Hermes for this project
          </Link>
        </label>
        <label>
          Global connection profile
          <NativeSelect
            aria-label="Global connection profile"
            value={globalConnectionID}
            onChange={(e) => setGlobalConnectionID(e.target.value)}
          >
            <option value="">Workspace-specific (blocked initially)</option>
            {globalConnections.data?.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </NativeSelect>
        </label>
        {parent && (
          <label>
            New local Git branch
            <Input
              value={branch}
              onChange={(event) => setBranch(event.target.value)}
              required
              maxLength={80}
              pattern="[A-Za-z][A-Za-z0-9/_-]{0,79}"
              placeholder="pr-42-gallery-layout"
            />
            <small>
              This creates a local branch. It does not fetch or publish a remote
              pull request.
            </small>
          </label>
        )}
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button variant="outline" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="default" disabled={mutation.isPending}>
            {mutation.isPending
              ? "Creating…"
              : parent
                ? "Create branch workspace"
                : "Create workspace"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
