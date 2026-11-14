import { expect, it, vi } from "vitest";
import type { KeyboardEvent } from "react";
import { composerKeyDown } from "./composer-keydown";
function key(key: string, shiftKey = false, isComposing = false) {
  return {
    key,
    shiftKey,
    nativeEvent: { isComposing },
    preventDefault: vi.fn(),
    currentTarget: { form: { requestSubmit: vi.fn() } },
  } as unknown as KeyboardEvent<HTMLTextAreaElement>;
}
it("preserves backward Tab navigation and native Shift+Enter newlines", () => {
  for (const name of ["Tab", "Enter"]) {
    const event = key(name, true);
    composerKeyDown(event, true);
    expect(event.preventDefault).not.toHaveBeenCalled();
    expect(event.currentTarget.form?.requestSubmit).not.toHaveBeenCalled();
  }
});
it("submits Enter only when ready and outside IME composition", () => {
  const event = key("Enter");
  composerKeyDown(event, true);
  expect(event.currentTarget.form?.requestSubmit).toHaveBeenCalledOnce();
  for (const [ready, composing] of [
    [false, false],
    [true, true],
  ]) {
    const blocked = key("Enter", false, composing);
    composerKeyDown(blocked, ready);
    expect(blocked.currentTarget.form?.requestSubmit).not.toHaveBeenCalled();
  }
});
