import { TicketMarkdown } from "./ticket-description";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { Modal, Form, ErrorNotice, useRequestKey } from "@/shared/components";
import { request } from "@/shared/http";
import { workKeys } from "@/features/work/api";
import { boardKey, type BoardCard } from "./board-api";

export function NewIssue({
  project,
  onClose,
  onCreated,
}: {
  project: string;
  onClose: () => void;
  onCreated: (id: string) => void;
}) {
  const client = useQueryClient();
  const key = useRequestKey();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [preview, setPreview] = useState(false);
  const [kind, setKind] = useState("software");
  const create = useMutation({
    mutationFn: () => {
      const payload = {
        project_id: project,
        title: title.trim(),
        description,
        kind,
      };
      return request<BoardCard>("/board", {
        ...payload,
        idempotency_key: key(payload),
      });
    },
    onSuccess: async (card) => {
      await Promise.all([
        client.invalidateQueries({ queryKey: boardKey(project) }),
        client.invalidateQueries({ queryKey: workKeys.list(project) }),
      ]);
      onCreated(card.id);
      onClose();
    },
  });
  return (
    <Modal
      title="New issue"
      onClose={() => {
        if (!create.isPending) onClose();
      }}
    >
      <Form onSubmit={() => create.mutate()}>
        <p className="text-sm text-muted-foreground">
          Add planned work to this project. No repository required.
        </p>
        <label>
          Title
          <Input
            autoFocus
            required
            maxLength={256}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            disabled={create.isPending}
          />
        </label>
        <div className="ticket-editor-tabs">
          <Button
            type="button"
            variant={preview ? "ghost" : "secondary"}
            aria-pressed={!preview}
            onClick={() => setPreview(false)}
          >
            Write
          </Button>
          <Button
            type="button"
            variant={preview ? "secondary" : "ghost"}
            aria-pressed={preview}
            onClick={() => setPreview(true)}
          >
            Preview
          </Button>
        </div>
        {preview ? (
          <div className="ticket-description-body">
            {description.trim() ? (
              <TicketMarkdown content={description} />
            ) : (
              <p>Nothing to preview.</p>
            )}
          </div>
        ) : (
          <label>
            Description
            <Textarea
              rows={5}
              maxLength={60000}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={create.isPending}
              placeholder="Describe the issue, steps to reproduce, and expected behaviour. Markdown is supported."
            />
          </label>
        )}
        <label>
          Type
          <NativeSelect
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            disabled={create.isPending}
          >
            <option value="software">Software</option>
            <option value="research">Research</option>
            <option value="digest">Digest</option>
          </NativeSelect>
        </label>
        <p className="text-xs text-muted-foreground">
          Saved in Planned. Work starts only when you ask the agent to run it.
        </p>
        {create.isError && <ErrorNotice error={create.error} />}
        <div className="flex gap-2">
          <Button disabled={!title.trim() || create.isPending}>
            {create.isPending ? "Creating…" : "Create issue"}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={create.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
