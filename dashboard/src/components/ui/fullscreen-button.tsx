import { useState, type RefObject } from "react";
import { Maximize, Minimize } from "lucide-react";
import { useFullscreenElement } from "@/shared/fullscreen";
import { Button } from "./button";

export function FullscreenButton({
  target,
  disabled,
}: {
  target: RefObject<HTMLElement | null>;
  disabled?: boolean;
}) {
  const fullscreenElement = useFullscreenElement();
  const [failed, setFailed] = useState(false);
  const active = !!fullscreenElement && fullscreenElement === target.current;
  const label = active ? "Exit fullscreen" : "Enter fullscreen";

  return (
    <>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={label}
        title={document.fullscreenEnabled ? label : "Fullscreen unavailable"}
        disabled={!active && (disabled || !document.fullscreenEnabled)}
        onClick={async () => {
          const element = target.current;
          if (!element) return;
          setFailed(false);
          try {
            if (active) await document.exitFullscreen();
            else await element.requestFullscreen();
          } catch {
            setFailed(true);
          }
        }}
      >
        {active ? <Minimize /> : <Maximize />}
      </Button>
      {failed && (
        <span role="status" className="text-xs">
          Could not change fullscreen mode.
        </span>
      )}
    </>
  );
}
