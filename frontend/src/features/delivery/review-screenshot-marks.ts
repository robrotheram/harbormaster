export type Point = { x: number; y: number };

export type ScreenshotMark = {
  tool: "highlight" | "arrow";
  start: Point;
  end: Point;
};

export function drawScreenshotMark(
  context: CanvasRenderingContext2D,
  mark: ScreenshotMark,
) {
  const weight = Math.max(
    2,
    Math.max(context.canvas.width, context.canvas.height) / 250,
  );

  if (mark.tool === "highlight") {
    const { start, end } = mark;
    const width = end.x - start.x;
    const height = end.y - start.y;
    context.fillStyle = "#facc1540";
    context.strokeStyle = "#3f2803";
    context.lineWidth = weight * 1.8;
    context.fillRect(start.x, start.y, width, height);
    context.strokeRect(start.x, start.y, width, height);
    context.strokeStyle = "#facc15";
    context.lineWidth = weight;
    context.strokeRect(start.x, start.y, width, height);
    return;
  }

  context.lineCap = "round";
  context.lineJoin = "round";
  context.beginPath();
  const { start, end } = mark;
  const distance = Math.hypot(end.x - start.x, end.y - start.y);
  const head = Math.min(weight * 5, distance * 0.45);
  const angle = Math.atan2(end.y - start.y, end.x - start.x);
  context.moveTo(start.x, start.y);
  context.lineTo(end.x, end.y);
  context.lineTo(
    end.x - head * Math.cos(angle - Math.PI / 6),
    end.y - head * Math.sin(angle - Math.PI / 6),
  );
  context.moveTo(end.x, end.y);
  context.lineTo(
    end.x - head * Math.cos(angle + Math.PI / 6),
    end.y - head * Math.sin(angle + Math.PI / 6),
  );

  context.strokeStyle = "#fff";
  context.lineWidth = weight * 1.8;
  context.stroke();
  context.strokeStyle = "#d92d20";
  context.lineWidth = weight;
  context.stroke();
}
