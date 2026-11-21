import { useEffect, useRef, useState, type RefObject } from "react";
import { Pencil, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { encodeScreenshot, screenshotCanvas } from "./review-screenshot-image";

export const maxReviewAreas = 6;
type Outline = { x: number; y: number; width: number; height: number };
export type ReviewArea = {
  id: string;
  path: string;
  url: string;
  outline: Outline;
  comment: string;
  service: string;
};

function validOutline(value: Outline | undefined): value is Outline {
  return (
    !!value &&
    [value.x, value.y, value.width, value.height].every(
      (n) => Number.isFinite(n) && n >= 0 && n <= 1,
    ) &&
    value.width > 0 &&
    value.height > 0 &&
    value.x + value.width <= 1.001 &&
    value.y + value.height <= 1.001
  );
}

export function ReviewPreviewMarkup({
  frame,
  origin,
  service,
  areas,
  disabled,
  onArea,
  onBusy,
}: {
  frame: RefObject<HTMLIFrameElement | null>;
  origin: string;
  service: string;
  areas: ReviewArea[];
  disabled: boolean;
  onArea: (area: ReviewArea) => void;
  onBusy: (busy: boolean) => void;
}) {
  const [active, setActive] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [nonce] = useState(() => crypto.randomUUID());
  const timeout = useRef<number>(undefined);
  const latest = useRef({ areas, onArea, onBusy });
  latest.current = { areas, onArea, onBusy };
  const placing = active && !disabled && areas.length < maxReviewAreas;

  useEffect(() => {
    function receive(event: MessageEvent) {
      if (
        event.source !== frame.current?.contentWindow ||
        event.origin !== origin ||
        event.data?.nonce !== nonce ||
        !placing
      )
        return;
      const value = event.data;
      if (value.type === "workbench-review-capturing") {
        window.clearTimeout(timeout.current);
        setBusy(true);
        latest.current.onBusy(true);
        timeout.current = window.setTimeout(() => {
          setBusy(false);
          latest.current.onBusy(false);
          setActive(false);
          setError(
            "The preview took too long to render. Try drawing the area again.",
          );
        }, 12000);
        return;
      }
      if (
        value.type !== "workbench-review-area" &&
        value.type !== "workbench-review-failed"
      )
        return;
      window.clearTimeout(timeout.current);
      setBusy(false);
      latest.current.onBusy(false);
      if (value.type === "workbench-review-failed") {
        setError("Could not render the preview area. Try drawing it again.");
        return;
      }
      if (
        typeof value.id !== "string" ||
        !/^[a-f0-9-]{36}$/.test(value.id) ||
        typeof value.path !== "string" ||
        !value.path.startsWith("/") ||
        value.path.length > 1024 ||
        typeof value.url !== "string" ||
        value.url.length > 1_398_100 ||
        !/^data:image\/jpeg;base64,[a-z0-9+/]+=*$/i.test(value.url) ||
        !validOutline(value.outline) ||
        latest.current.areas.length >= maxReviewAreas ||
        latest.current.areas.some((area) => area.id === value.id)
      ) {
        setError("The preview returned an invalid area. Draw it again.");
        return;
      }
      latest.current.onArea({
        id: value.id,
        path: value.path,
        url: value.url,
        outline: value.outline,
        service,
        comment: "",
      });
      setActive(false);
    }
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, [frame, origin, nonce, placing, service]);

  useEffect(
    () => () => {
      window.clearTimeout(timeout.current);
      latest.current.onBusy(false);
    },
    [],
  );

  const visible = areas.flatMap(({ id, service: source }, i) =>
    source === service ? [{ id, number: i + 1 }] : [],
  );
  const areaKey = JSON.stringify(visible);
  useEffect(() => {
    const connection = frame.current;
    const sync = () =>
      connection?.contentWindow?.postMessage(
        {
          type: "workbench-review",
          nonce,
          enabled: placing,
          areas: JSON.parse(areaKey),
        },
        origin,
      );
    sync();
    connection?.addEventListener("load", sync);
    return () => connection?.removeEventListener("load", sync);
  }, [frame, origin, nonce, placing, areaKey]);
  useEffect(() => {
    const connection = frame.current;
    return () => {
      const visible = latest.current.areas.flatMap(
        ({ id, service: source }, i) =>
          source === service ? [{ id, number: i + 1 }] : [],
      );
      connection?.contentWindow?.postMessage(
        { type: "workbench-review", nonce, enabled: false, areas: visible },
        origin,
      );
    };
  }, [frame, origin, nonce, service]);

  const hint = busy
    ? "Rendering area…"
    : placing
      ? "Drag a rectangle on the preview."
      : areas.length >= maxReviewAreas
        ? "Six areas added."
        : "";
  return (
    <div className="review-preview-controls">
      <Button
        type="button"
        variant={placing ? "secondary" : "ghost"}
        size="sm"
        title={hint || "Draw an area on the preview and add feedback"}
        aria-pressed={placing}
        disabled={disabled || busy || areas.length >= maxReviewAreas}
        onClick={() => {
          setError("");
          setActive(!active);
        }}
      >
        <Pencil /> Markup
      </Button>
      {(error || hint) && (
        <span className="review-preview-hint" role={error ? "alert" : "status"}>
          {error || hint}
        </span>
      )}
    </div>
  );
}

export function AreaComments({
  areas,
  disabled,
  onChange,
}: {
  areas: ReviewArea[];
  disabled: boolean;
  onChange: (areas: ReviewArea[]) => void;
}) {
  return (
    <ol className="review-area-comments" aria-label="Preview changes">
      {areas.map((area, index) => (
        <li key={area.id}>
          <img src={area.url} alt="" />
          <label>
            <strong>Area {index + 1}</strong>
            <Textarea
              placeholder="Describe what should change in this area…"
              value={area.comment}
              maxLength={280}
              disabled={disabled}
              onChange={(event) =>
                onChange(
                  areas.map((item) =>
                    item.id === area.id
                      ? { ...item, comment: event.target.value }
                      : item,
                  ),
                )
              }
            />
          </label>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            disabled={disabled}
            aria-label={`Remove area ${index + 1}`}
            onClick={() => onChange(areas.filter(({ id }) => id !== area.id))}
          >
            <X />
          </Button>
        </li>
      ))}
    </ol>
  );
}

function wrapComment(
  context: CanvasRenderingContext2D,
  comment: string,
  width: number,
) {
  const lines: string[] = [];
  let line = "";
  for (const word of comment.trim().split(/\s+/)) {
    if (line && context.measureText(line + " " + word).width > width) {
      lines.push(line);
      line = "";
    }
    if (line) line += " ";
    for (const char of word) {
      if (line && context.measureText(line + char).width > width) {
        lines.push(line);
        line = "";
      }
      line += char;
    }
  }
  if (line) lines.push(line);
  return lines;
}

export async function renderReviewAreas(areas: ReviewArea[]) {
  const width = 720;
  const gap = 16;
  const captionHeight = 26;
  const columns = Math.min(2, areas.length);
  const { canvas, context } = screenshotCanvas(
    columns * width + (columns + 1) * gap,
    1,
  );
  const font = "23px system-ui, sans-serif";
  context.font = font;
  const panels = await Promise.all(
    areas.map(async (area) => {
      const image = new Image();
      image.src = area.url;
      await image.decode();
      const ratio = Math.min(
        width / image.naturalWidth,
        390 / image.naturalHeight,
      );
      const lines = wrapComment(context, area.comment, width - 32);
      return {
        image,
        lines,
        outline: area.outline,
        imageWidth: image.naturalWidth * ratio,
        imageHeight: image.naturalHeight * ratio,
      };
    }),
  );
  const rows = Array.from(
    { length: Math.ceil(panels.length / columns) },
    (_, row) => panels.slice(row * columns, (row + 1) * columns),
  );
  const heights = rows.map((row) =>
    Math.max(
      ...row.map(
        (panel) => panel.imageHeight + panel.lines.length * captionHeight + 54,
      ),
    ),
  );
  const height = Math.ceil(heights.reduce((sum, row) => sum + row + gap, gap));
  const scale = Math.min(1, 1920 / height);
  canvas.width = Math.round(canvas.width * scale);
  canvas.height = Math.round(height * scale);
  context.scale(scale, scale);
  context.fillStyle = "#eef1f6";
  context.fillRect(0, 0, canvas.width / scale, height);
  let y = gap;
  rows.forEach((row, index) => {
    row.forEach(
      ({ image, imageWidth, imageHeight, lines, outline }, column) => {
        const number = index * columns + column + 1;
        const x = gap + column * (width + gap);
        context.fillStyle = "#fff";
        context.fillRect(x, y, width, heights[index]);
        const left = x + (width - imageWidth) / 2;
        context.drawImage(image, left, y, imageWidth, imageHeight);
        const markX = left + outline.x * imageWidth;
        const markY = y + outline.y * imageHeight;
        context.lineWidth = 4;
        context.strokeStyle = "#fff";
        context.strokeRect(
          markX,
          markY,
          outline.width * imageWidth,
          outline.height * imageHeight,
        );
        context.lineWidth = 2;
        context.strokeStyle = "#254ade";
        context.strokeRect(
          markX,
          markY,
          outline.width * imageWidth,
          outline.height * imageHeight,
        );
        context.fillStyle = "#254ade";
        context.fillRect(markX, markY, 30, 30);
        context.font = "bold 20px system-ui, sans-serif";
        context.fillStyle = "#fff";
        context.fillText(String(number), markX + 8, markY + 22);
        context.fillStyle = "#152349";
        context.fillText(`Area ${number}`, x + 16, y + imageHeight + 27);
        context.font = font;
        lines.forEach((line, i) =>
          context.fillText(
            line,
            x + 16,
            y + imageHeight + 54 + i * captionHeight,
          ),
        );
      },
    );
    y += heights[index] + gap;
  });
  return encodeScreenshot(canvas);
}
