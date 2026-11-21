import { useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Modal } from "@/shared/components";
import { encodeScreenshot, screenshotCanvas } from "./review-screenshot-image";
import {
  drawScreenshotMark,
  type ScreenshotMark,
} from "./review-screenshot-marks";

import { useReviewDrawing } from "./review-drawing";

export type ReviewScreenshot = {
  url: string;
  note: string;
  markup?: { source: string; marks: ScreenshotMark[] };
};

export function ReviewScreenshotEditor({
  shot,
  disabled,
  returnFocus,
  onClose,
  onSave,
}: {
  shot: ReviewScreenshot;
  disabled: boolean;
  returnFocus: HTMLElement | null;
  onClose: () => void;
  onSave: (shot: ReviewScreenshot) => void;
}) {
  const source = shot.markup?.source || shot.url;
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });
  const [error, setError] = useState("");
  const image = useRef<HTMLImageElement>(null);
  const { marks, drawing, toolbar, layer, onKeyDown } = useReviewDrawing(
    dimensions,
    shot.markup?.marks,
  );

  function save() {
    if (!image.current || disabled || drawing.current) return;
    try {
      let next = shot;
      if (!marks.length && shot.markup) {
        next = { url: source, note: shot.note };
      } else if (marks.length && marks !== shot.markup?.marks) {
        const output = screenshotCanvas(dimensions.width, dimensions.height);
        output.context.drawImage(image.current, 0, 0);
        for (const mark of marks) drawScreenshotMark(output.context, mark);
        next = {
          url: encodeScreenshot(output.canvas),
          note: shot.note,
          markup: { source, marks },
        };
      }
      onSave(next);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to save the markup.");
    }
  }

  return (
    <Modal
      title="Mark up screenshot"
      className="gap-3 p-3 sm:max-w-[1100px] sm:p-5"
      onClose={onClose}
      onCloseAutoFocus={(event) => {
        const target = returnFocus;
        if (target?.isConnected) {
          event.preventDefault();
          requestAnimationFrame(() => target.focus());
        }
      }}
    >
      <div className="review-markup" onKeyDown={onKeyDown}>
        {toolbar}
        <div className="review-markup-stage">
          <div className="review-markup-image">
            <img
              src={source}
              ref={image}
              alt="Screenshot being marked up"
              draggable={false}
              onLoad={(event) =>
                setDimensions({
                  width: event.currentTarget.naturalWidth,
                  height: event.currentTarget.naturalHeight,
                })
              }
              onError={() => setError("Unable to open this screenshot.")}
            />
            {layer}
          </div>
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <div className="review-markup-actions">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            disabled={!dimensions.width || disabled}
            onClick={save}
          >
            Save markup
          </Button>
        </div>
      </div>
    </Modal>
  );
}
