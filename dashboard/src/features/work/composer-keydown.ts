import type { KeyboardEvent } from "react";

export function composerKeyDown(
  event: KeyboardEvent<HTMLTextAreaElement>,
  canSend: boolean,
) {
  if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229)
    return;
  if (event.ctrlKey || event.altKey || event.metaKey) return;

  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    if (canSend && !event.repeat) event.currentTarget.form?.requestSubmit();
  }
}
