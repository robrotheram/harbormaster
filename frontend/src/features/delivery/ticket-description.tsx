import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ErrorNotice } from "@/shared/components";
import { instructionOptions } from "@/features/work/conversation-title";
import { userMessage } from "@/features/work/conversation-api";
import type { BoardCard } from "./board-api";

export function TicketMarkdown({ content }: { content: string }) {
  return (
    <div className="message-markdown ticket-markdown">
      <Markdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        components={{
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          ),
          img: ({ src, alt }) => (
            <a href={src} target="_blank" rel="noopener noreferrer">
              {alt || "Open image"}
            </a>
          ),
        }}
      >
        {content}
      </Markdown>
    </div>
  );
}
export function TicketDescription({
  card,
  busy,
  onSave,
}: {
  card: BoardCard;
  busy: boolean;
  onSave: (description: string) => Promise<unknown>;
}) {
  const instruction = useQuery({
    ...instructionOptions(card.project_id, card.work),
    enabled: card.description === undefined,
  });
  const fallback = instruction.data
    ? userMessage(instruction.data, { parent_attempt_id: "followup" })
        .replace(/^# [^\n]+\n\n/, "")
        .replace(/^Source: [^\n]+\nRepository: [^\n]+\n\n/, "")
    : "";
  const content = card.description ?? fallback;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [preview, setPreview] = useState(false);
  async function save() {
    try {
      await onSave(draft);
      setEditing(false);
    } catch {
      /* The enclosing panel displays the error and keeps the draft. */
    }
  }
  return (
    <section className="ticket-description" aria-label="Issue description">
      <header>
        <h3>Description</h3>
        {!editing && (
          <Button
            size="sm"
            variant="ghost"
            disabled={
              busy || (card.description === undefined && instruction.isPending)
            }
            onClick={() => {
              setDraft(content);
              setPreview(false);
              setEditing(true);
            }}
          >
            Edit description
          </Button>
        )}
      </header>
      {card.description === undefined && instruction.isError && (
        <ErrorNotice error={instruction.error} />
      )}
      {editing ? (
        <>
          <div className="ticket-editor-tabs">
            <Button
              variant={!preview ? "secondary" : "ghost"}
              aria-pressed={!preview}
              onClick={() => setPreview(false)}
            >
              Write
            </Button>
            <Button
              variant={preview ? "secondary" : "ghost"}
              aria-pressed={preview}
              onClick={() => setPreview(true)}
            >
              Preview
            </Button>
          </div>
          <div className="ticket-description-body">
            {preview ? (
              draft.trim() ? (
                <TicketMarkdown content={draft} />
              ) : (
                <p className="text-muted-foreground">Nothing to preview.</p>
              )
            ) : (
              <Textarea
                aria-label="Issue description"
                value={draft}
                disabled={busy}
                rows={12}
                maxLength={60000}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="Describe the issue, expected behaviour, and steps to reproduce. Markdown is supported."
              />
            )}
          </div>
          <footer>
            <span>Markdown supported · use - [ ] for tasks</span>
            <div className="flex gap-2">
              <Button disabled={busy || draft === content} onClick={save}>
                Save description
              </Button>
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => setEditing(false)}
              >
                Cancel
              </Button>
            </div>
          </footer>
        </>
      ) : (
        <div className="ticket-description-body">
          {card.description === undefined && instruction.isPending ? (
            <p role="status">Loading description…</p>
          ) : content.trim() ? (
            <TicketMarkdown content={content} />
          ) : (
            <p className="text-muted-foreground">No description provided.</p>
          )}
        </div>
      )}
    </section>
  );
}
