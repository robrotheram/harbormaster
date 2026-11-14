import { workspacesApi } from "./api";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";

import { label } from "@/shared/format";
import { ErrorNotice, Form, Modal } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import { NativeSelect } from "@/components/ui/native-select";

import type { Project, Workspace } from "@/shared/types";
export function CopyForm({
  project,
  workspaces,
  onClose,
  onCopied,
}: {
  project: Project;
  workspaces: Workspace[];
  onClose: () => void;
  onCopied: (size: number) => void;
}) {
  const [source, setSource] = useState(""),
    [destination, setDestination] = useState("");
  const [sourcePath, setSourcePath] = useState(""),
    [destinationPath, setDestinationPath] = useState("");
  const [localError, setLocalError] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      workspacesApi.handoff(
        source,
        sourcePath.trim(),
        destination,
        destinationPath.trim(),
      ),
    onSuccess: (result) => onCopied(result.bytes),
  });
  const canCopy =
    !mutation.isPending &&
    !!source &&
    !!destination &&
    source !== destination &&
    !!sourcePath.trim() &&
    !!destinationPath.trim();
  return (
    <Modal
      title="Copy a project file"
      onClose={onClose}
      closeDisabled={mutation.isPending}
    >
      <Form
        onSubmit={() => {
          if (mutation.isPending) return;
          if (source === destination) {
            setLocalError("Choose two different workers.");
            return;
          }
          setLocalError("");
          if (canCopy) mutation.mutate();
        }}
      >
        <fieldset disabled={mutation.isPending} className="min-w-0 space-y-5">
          <div className="form-project">
            Project <strong>{project.name}</strong>
          </div>
          <p className="form-note">
            Both workers must be running and idle. Copy one regular file up to 1
            MiB. Paths are relative to /data/files. Choose a new destination
            name; copies never overwrite files.
          </p>
          <label>
            From worker
            <NativeSelect
              aria-label="From worker"
              required
              value={source}
              onChange={(event) => {
                setSource(event.target.value);
                if (event.target.value === destination) setDestination("");
                setLocalError("");
              }}
            >
              <option value="">Choose source</option>
              {workspaces.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name || w.id} ({label(w.state)})
                </option>
              ))}
            </NativeSelect>
          </label>
          <label>
            Source file
            <Input
              required
              maxLength={512}
              value={sourcePath}
              onChange={(event) => setSourcePath(event.target.value)}
              placeholder="reports/findings.md"
            />
          </label>
          <label>
            To worker
            <NativeSelect
              aria-label="To worker"
              required
              value={destination}
              onChange={(event) => setDestination(event.target.value)}
            >
              <option value="">Choose destination</option>
              {workspaces
                .filter((w) => w.id !== source)
                .map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.name || w.id} ({label(w.state)})
                  </option>
                ))}
            </NativeSelect>
          </label>
          <label>
            New destination file
            <Input
              required
              maxLength={512}
              value={destinationPath}
              onChange={(event) => setDestinationPath(event.target.value)}
              placeholder="incoming/findings.md"
            />
          </label>
          {localError && (
            <div className="notice error" role="alert">
              {localError}
            </div>
          )}
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          <div className="form-actions">
            <Button variant="outline" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="default" disabled={!canCopy}>
              {mutation.isPending ? "Copying…" : "Copy file"}
            </Button>
          </div>
        </fieldset>
      </Form>
    </Modal>
  );
}
