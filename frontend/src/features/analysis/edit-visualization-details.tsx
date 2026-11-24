import { useId, useState } from "react";
import { Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ErrorNotice, Form, Modal } from "@/shared/components";
import { useUpdatePersonalVisualization } from "./use-visualization-details";

const fieldClassName =
  "bg-background px-3.5 py-2.5 leading-6 shadow-none focus-visible:ring-2 dark:bg-background";

export function EditVisualizationDetails({
  project,
  worker,
  id,
  title,
  description,
  disabled,
}: {
  project: string;
  worker: string;
  id: string;
  title: string;
  description: string;
  disabled: boolean;
}) {
  const fieldId = useId();
  const [draft, setDraft] = useState<{ title: string; description: string }>();
  const update = useUpdatePersonalVisualization(project);
  const nextTitle = draft?.title.trim() ?? "";
  const nextDescription = draft?.description.trim() ?? "";
  const titleTooLong = [...nextTitle].length > 200;
  const descriptionTooLong = [...nextDescription].length > 2000;
  const canSubmit =
    !!nextTitle && !titleTooLong && !descriptionTooLong && !update.isPending;
  return (
    <>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label="Edit visualization details"
        title="Edit visualization details"
        disabled={disabled}
        onClick={() => {
          update.reset();
          setDraft({ title, description });
        }}
      >
        <Pencil className="size-3.5" />
      </Button>
      {draft !== undefined && (
        <Modal
          title="Edit visualization details"
          onClose={() => setDraft(undefined)}
          closeDisabled={update.isPending}
          className="bg-card p-5 sm:max-w-xl sm:p-6"
        >
          <Form
            onSubmit={() => {
              if (!canSubmit) return;
              const titleChanged = nextTitle !== title;
              const descriptionChanged = nextDescription !== description.trim();
              if (!titleChanged && !descriptionChanged) {
                setDraft(undefined);
                return;
              }
              update.mutate(
                {
                  worker_id: worker,
                  artifact_id: id,
                  ...(titleChanged ? { title: nextTitle } : {}),
                  ...(descriptionChanged
                    ? { description: nextDescription }
                    : {}),
                },
                { onSuccess: () => setDraft(undefined) },
              );
            }}
          >
            <div className="grid gap-4">
              <div className="grid gap-2">
                <label htmlFor={`${fieldId}-title`}>Title</label>
                <Input
                  id={`${fieldId}-title`}
                  className={`${fieldClassName} h-12`}
                  value={draft.title}
                  onChange={(event) =>
                    setDraft({ ...draft, title: event.target.value })
                  }
                  autoFocus
                  disabled={update.isPending}
                  aria-invalid={titleTooLong || undefined}
                  aria-describedby={
                    titleTooLong ? `${fieldId}-title-error` : undefined
                  }
                  required
                />
                {titleTooLong && (
                  <p
                    id={`${fieldId}-title-error`}
                    role="alert"
                    className="text-sm text-destructive"
                  >
                    Use 200 characters or fewer for the title.
                  </p>
                )}
              </div>
              <div className="grid gap-2">
                <label htmlFor={`${fieldId}-description`}>Description</label>
                <Textarea
                  id={`${fieldId}-description`}
                  className={`${fieldClassName} block field-sizing-fixed max-h-64 resize-y`}
                  rows={4}
                  value={draft.description}
                  onChange={(event) =>
                    setDraft({ ...draft, description: event.target.value })
                  }
                  disabled={update.isPending}
                  aria-invalid={descriptionTooLong || undefined}
                  aria-describedby={
                    descriptionTooLong
                      ? `${fieldId}-description-error`
                      : undefined
                  }
                />
                {descriptionTooLong && (
                  <p
                    id={`${fieldId}-description-error`}
                    role="alert"
                    className="text-sm text-destructive"
                  >
                    Use 2,000 characters or fewer for the description.
                  </p>
                )}
              </div>
            </div>
            {update.isError && <ErrorNotice error={update.error} />}
            <div className="form-actions pt-4">
              <Button
                type="button"
                variant="outline"
                className="min-h-10 px-4"
                onClick={() => setDraft(undefined)}
                disabled={update.isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                className="min-h-10 px-4"
                disabled={!canSubmit}
              >
                Save changes
              </Button>
            </div>
          </Form>
        </Modal>
      )}
    </>
  );
}
