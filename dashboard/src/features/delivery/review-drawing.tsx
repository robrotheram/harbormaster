import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import { ArrowUpRight, Highlighter, Redo2, Undo2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  drawScreenshotMark,
  type Point,
  type ScreenshotMark,
} from "./review-screenshot-marks";

const tools = [
  { id: "highlight", label: "Highlight", icon: Highlighter },
  { id: "arrow", label: "Arrow", icon: ArrowUpRight },
] as const;

export function useReviewDrawing(
  dimensions: { width: number; height: number },
  initialMarks: ScreenshotMark[] = [],
  label = "Screenshot drawing layer",
) {
  const [tool, setTool] = useState<ScreenshotMark["tool"]>("highlight");
  const [marks, setMarks] = useState(initialMarks);
  const [undone, setUndone] = useState<ScreenshotMark[]>([]);
  const canvas = useRef<HTMLCanvasElement>(null);
  const drawing = useRef<{ pointer: number; mark: ScreenshotMark }>(null);

  function repaint() {
    const context = canvas.current?.getContext("2d");
    if (!context) return;
    context.clearRect(0, 0, dimensions.width, dimensions.height);
    for (const mark of marks) drawScreenshotMark(context, mark);
    if (drawing.current) drawScreenshotMark(context, drawing.current.mark);
  }
  useEffect(repaint, [marks, dimensions]);

  function undo() {
    const last = marks.at(-1);
    if (!last || drawing.current) return;
    setMarks(marks.slice(0, -1));
    setUndone([...undone, last]);
  }

  function redo() {
    const last = undone.at(-1);
    if (!last || drawing.current) return;
    setUndone(undone.slice(0, -1));
    setMarks([...marks, last]);
  }

  function position(event: PointerEvent<HTMLCanvasElement>): Point {
    const rect = event.currentTarget.getBoundingClientRect();
    return {
      x:
        Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width)) *
        dimensions.width,
      y:
        Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height)) *
        dimensions.height,
    };
  }

  function start(event: PointerEvent<HTMLCanvasElement>) {
    if (event.button !== 0 || drawing.current) return;
    const point = position(event);
    event.currentTarget.setPointerCapture(event.pointerId);
    drawing.current = {
      pointer: event.pointerId,
      mark: { tool, start: point, end: point },
    };
    repaint();
  }

  function move(event: PointerEvent<HTMLCanvasElement>) {
    const active = drawing.current;
    if (active?.pointer !== event.pointerId) return;
    active.mark.end = position(event);
    repaint();
  }

  function finish(event: PointerEvent<HTMLCanvasElement>) {
    const active = drawing.current;
    if (active?.pointer !== event.pointerId) return;
    move(event);
    drawing.current = null;
    const { mark } = active;
    if (mark.start.x === mark.end.x && mark.start.y === mark.end.y) {
      repaint();
      return;
    }
    setMarks([...marks, mark]);
    setUndone([]);
  }

  function cancel(event: PointerEvent<HTMLCanvasElement>) {
    if (drawing.current?.pointer !== event.pointerId) return;
    drawing.current = null;
    repaint();
  }

  function onKeyDown(event: KeyboardEvent) {
    if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
    const key = event.key.toLowerCase();
    if (key !== "z" && key !== "y") return;
    event.preventDefault();
    if (key === "y" || event.shiftKey) redo();
    else undo();
  }

  const toolbar = (
    <div className="review-markup-toolbar">
      <div role="group" aria-label="Drawing tools">
        {tools.map(({ id, label, icon: Icon }) => (
          <Button
            key={id}
            type="button"
            variant="ghost"
            aria-pressed={id === tool}
            onClick={() => setTool(id)}
          >
            <Icon /> {label}
          </Button>
        ))}
      </div>
      <div role="group" aria-label="Drawing history">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Undo"
          title="Undo"
          disabled={!marks.length}
          onClick={undo}
        >
          <Undo2 />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Redo"
          title="Redo"
          disabled={!undone.length}
          onClick={redo}
        >
          <Redo2 />
        </Button>
      </div>
    </div>
  );
  const layer = dimensions.width > 0 && (
    <canvas
      ref={canvas}
      width={dimensions.width}
      height={dimensions.height}
      role="img"
      aria-label={label}
      onPointerDown={start}
      onPointerMove={move}
      onPointerUp={finish}
      onPointerCancel={cancel}
      onLostPointerCapture={cancel}
    />
  );
  return { marks, drawing, toolbar, layer, onKeyDown };
}
