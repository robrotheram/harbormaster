import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ReviewScreenshot } from "./review-screenshot-editor";

export function ReviewScreenshots({
  shots,
  onRemove,
  onEdit,
  disabled,
}: {
  shots: ReviewScreenshot[];
  onRemove: (index: number) => void;
  onEdit: (index: number, trigger: HTMLElement) => void;
  disabled: boolean;
}) {
  return (
    <ul
      aria-label="Attached screenshots"
      className="flex flex-wrap gap-2 px-3 pt-3"
    >
      {shots.map((shot, index) => (
        <li key={index} className="relative">
          <button
            type="button"
            className="block overflow-hidden rounded-md border bg-muted focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-60"
            aria-label={`Mark up screenshot ${index + 1}`}
            disabled={disabled}
            onClick={(event) => onEdit(index, event.currentTarget)}
          >
            <img
              src={shot.url}
              alt={`Review screenshot ${index + 1}`}
              className="h-14 w-20 object-contain"
            />
          </button>
          <Button
            type="button"
            size="icon-xs"
            variant="secondary"
            className="absolute top-0.5 right-0.5"
            aria-label={`Remove screenshot ${index + 1}`}
            disabled={disabled}
            onClick={() => onRemove(index)}
          >
            <X />
          </Button>
        </li>
      ))}
    </ul>
  );
}
