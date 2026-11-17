import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

export type ReviewScreenshot = { content: string; note: string; url: string };

export function ReviewScreenshots({
  shots,
  onChange,
  disabled,
  onBusy,
}: {
  shots: ReviewScreenshot[];
  onChange: (shots: ReviewScreenshot[]) => void;
  disabled: boolean;
  onBusy: (busy: boolean) => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function add(blob: Blob) {
    if (!["image/png", "image/jpeg"].includes(blob.type))
      throw new Error("Choose a PNG or JPEG screenshot.");
    const bitmap = await createImageBitmap(blob);
    try {
      const scale = Math.min(1, 1920 / Math.max(bitmap.width, bitmap.height));
      const canvas = document.createElement("canvas");
      canvas.width = Math.max(1, Math.round(bitmap.width * scale));
      canvas.height = Math.max(1, Math.round(bitmap.height * scale));
      const ctx = canvas.getContext("2d")!;
      ctx.fillStyle = "#fff";
      ctx.fillRect(0, 0, canvas.width, canvas.height);
      ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
      const url = canvas.toDataURL("image/jpeg", 0.85);
      const content = url.split(",")[1];
      if (content.length > 1398100)
        throw new Error("Screenshot is too large. Crop it and try again.");
      onChange([...shots, { content, url, note: "" }]);
    } finally {
      bitmap.close();
    }
  }
  async function run(action: () => Promise<void>) {
    setBusy(true);
    onBusy(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Unable to capture screenshot.",
      );
    } finally {
      setBusy(false);
      onBusy(false);
    }
  }
  async function capture() {
    if (!navigator.mediaDevices?.getDisplayMedia)
      throw new Error(
        "Screen capture is unavailable in this browser. Attach a screenshot instead.",
      );
    const stream = await navigator.mediaDevices.getDisplayMedia({
      video: true,
      audio: false,
    });
    const video = document.createElement("video");
    try {
      video.srcObject = stream;
      await video.play();
      await new Promise<void>((resolve) =>
        video.requestVideoFrameCallback(() => resolve()),
      );
      const canvas = document.createElement("canvas");
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      canvas.getContext("2d")!.drawImage(video, 0, 0);
      const blob = await new Promise<Blob>((resolve, reject) =>
        canvas.toBlob(
          (b) => (b ? resolve(b) : reject(new Error("Capture failed."))),
          "image/png",
        ),
      );
      await add(blob);
    } finally {
      stream.getTracks().forEach((track) => track.stop());
      video.srcObject = null;
    }
  }
  const locked = disabled || busy || shots.length >= 4;
  return (
    <fieldset disabled={disabled || busy} className="review-screenshots">
      <legend>Screenshots and notes</legend>
      <div className="flex gap-2 items-center">
        <Button
          type="button"
          variant="outline"
          disabled={locked}
          onClick={() => void run(capture)}
        >
          Capture screenshot
        </Button>
        <label>
          Attach screenshot
          <input
            type="file"
            accept="image/png,image/jpeg"
            disabled={locked}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (file) void run(() => add(file));
            }}
          />
        </label>
      </div>
      <p>
        Choose a tab, window or screen. Up to four screenshots; images are
        resized to 1920 pixels.
      </p>
      {busy && <p role="status">Preparing screenshot…</p>}
      {error && <p role="alert">{error}</p>}
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
            onChange={(e) =>
              onChange(
                shots.map((s, i) =>
                  i === index ? { ...s, note: e.target.value } : s,
                ),
              )
            }
          />
          <Button
            type="button"
            variant="ghost"
            onClick={() => onChange(shots.filter((_, i) => i !== index))}
          >
            Remove screenshot {index + 1}
          </Button>
        </div>
      ))}
    </fieldset>
  );
}
