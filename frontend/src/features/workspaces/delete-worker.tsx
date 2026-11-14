import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorNotice, Form, Modal } from "@/shared/components";
import type { Workspace } from "@/shared/types";
import { workspacesApi } from "./api";

export function DeleteWorker({
  workspace,
  onClose,
  onDeleted,
}: {
  workspace: Workspace;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const [confirmation, setConfirmation] = useState("");
  const name = workspace.name || workspace.id;
  const deletion = useMutation({
    mutationFn: () => workspacesApi.delete(workspace.id),
    onSuccess: onDeleted,
  });
  const stopped = workspace.state === "STOPPED";
  const canDelete = confirmation === name && !deletion.isPending;
  return (
    <Modal
      title="Delete worker?"
      onClose={onClose}
      closeDisabled={deletion.isPending}
    >
      <Form
        onSubmit={() => {
          if (canDelete) deletion.mutate();
        }}
      >
        <p className="text-sm">
          Remove <strong className="break-words">{name}</strong> from Workbench?
          Private disks remain on the host. This worker will no longer appear in
          the inventory.
        </p>
        {!stopped && (
          <p role="status" className="notice">
            Deleting stops this worker first. Active work and open terminals
            must finish before deletion can succeed.
          </p>
        )}
        <label>
          Type the worker name to confirm
          <Input
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            disabled={deletion.isPending}
            autoComplete="off"
            spellCheck={false}
          />
        </label>
        {deletion.isError && <ErrorNotice error={deletion.error} />}
        <div className="form-actions">
          <Button
            type="button"
            variant="outline"
            disabled={deletion.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button variant="destructive" disabled={!canDelete}>
            {deletion.isPending ? "Deleting…" : "Delete worker"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
