import { Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { ReviewScreenshot } from "./review-screenshot-editor";
import { maxReviewScreenshots } from "./review-screenshot-image";

export function ReviewScreenshots({
  shots,
  limit = maxReviewScreenshots,
  onChange,
  onAttach,
  onEdit,
  disabled,
}: {
  shots: ReviewScreenshot[];
  limit?: number;
  onChange: (shots: ReviewScreenshot[]) => void;
  onAttach: (file: File) => void;
  onEdit: (index: number, trigger: HTMLElement) => void;
  disabled: boolean;
}) {
  const locked = disabled || shots.length >= limit;
  return (
    <fieldset disabled={disabled} className="review-screenshots">
      <legend>Screenshots and notes</legend>
      <div className="flex gap-2 items-center">
        <label>
          Attach screenshot
          <input
            type="file"
            accept="image/png,image/jpeg"
            disabled={locked}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (file) onAttach(file);
            }}
          />
        </label>
      </div>
      <p>Attach up to {limit} PNG or JPEG screenshots.</p>
      {shots.map((shot, index) => (
        <div key={index} className="review-screenshot">
          <img
            src={shot.url}
            alt={`Review screenshot ${index + 1}`}
            style={{ maxHeight: 180, maxWidth: "100%" }}
          />
          <Textarea
            aria-label={`Screenshot ${index + 1} notes`}
            placeholder="What should change here?"
            value={shot.note}
            maxLength={500}
            onChange={(event) =>
              onChange(
                shots.map((s, i) =>
                  i === index ? { ...s, note: event.target.value } : s,
                ),
              )
            }
          />
          <div className="review-screenshot-actions">
            <Button
              type="button"
              variant="outline"
              aria-label={`Mark up screenshot ${index + 1}`}
              onClick={(event) => onEdit(index, event.currentTarget)}
            >
              <Pencil /> Mark up
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onChange(shots.filter((_, i) => i !== index))}
            >
              Remove screenshot {index + 1}
            </Button>
          </div>
        </div>
      ))}
    </fieldset>
  );
}
