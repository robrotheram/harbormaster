const maxDimension = 1920;
export const maxReviewScreenshots = 4;

export function screenshotCanvas(width: number, height: number) {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("The screenshot canvas is unavailable.");
  return { canvas, context };
}

export function encodeScreenshot(canvas: HTMLCanvasElement) {
  for (const quality of [0.85, 0.7, 0.55]) {
    const url = canvas.toDataURL("image/jpeg", quality);
    if (url.length - url.indexOf(",") - 1 <= 1_398_100) return url;
  }
  throw new Error("Screenshot is too large. Crop it and try again.");
}

function prepareScreenshot(
  source: CanvasImageSource,
  width: number,
  height: number,
) {
  if (!width || !height) throw new Error("No screenshot frame was available.");
  const scale = Math.min(1, maxDimension / Math.max(width, height));
  const { canvas, context } = screenshotCanvas(
    Math.max(1, Math.round(width * scale)),
    Math.max(1, Math.round(height * scale)),
  );
  context.fillStyle = "#fff";
  context.fillRect(0, 0, canvas.width, canvas.height);
  context.drawImage(source, 0, 0, canvas.width, canvas.height);
  return encodeScreenshot(canvas);
}

export async function readScreenshot(blob: Blob) {
  if (!["image/png", "image/jpeg"].includes(blob.type))
    throw new Error("Choose a PNG or JPEG screenshot.");
  const bitmap = await createImageBitmap(blob);
  try {
    return prepareScreenshot(bitmap, bitmap.width, bitmap.height);
  } finally {
    bitmap.close();
  }
}
