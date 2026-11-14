import { expect, it } from "vitest";
import { conversationTitle } from "./conversation-title";

const item = {
  id: "work_12345678",
  specification_path: "work-requests/request.md",
};
it("uses the original request, excluding generated presentation instructions", () => {
  expect(
    conversationTitle(
      item,
      "Workbench presentation: interactive-v1.\nChart support.\n\nCurrent request:\nBuild a gallery\nUse blue accents.",
    ),
  ).toBe("Build a gallery");
  expect(conversationTitle(item, "\n# Fix the login form\nMore details")).toBe(
    "Fix the login form",
  );
});
it("bounds long titles and keeps unavailable conversations distinguishable", () => {
  expect(conversationTitle(item, "x".repeat(200))).toHaveLength(118);
  expect(conversationTitle(item)).toBe("Conversation 12345678");
  expect(
    conversationTitle({
      ...item,
      specification_path: "specifications/gallery.md",
    }),
  ).toBe("gallery.md");
});
it("uses the current request from a generated follow-up specification", () => {
  expect(
    conversationTitle(
      item,
      'Continue the same work in this workspace. Read the previous request at "work-requests/old.md" for context.\n\nCurrent request:\nFix checkout validation',
    ),
  ).toBe("Fix checkout validation");
});
