import { useId, useRef } from "react";
import {
  Bold,
  Code,
  Heading2,
  Italic,
  Link,
  List,
  ListOrdered,
  Quote,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

const inlineFormats = [
  {
    label: "Bold",
    Icon: Bold,
    before: "**",
    after: "**",
    placeholder: "bold text",
  },
  {
    label: "Italic",
    Icon: Italic,
    before: "*",
    after: "*",
    placeholder: "italic text",
  },
  {
    label: "Link",
    Icon: Link,
    before: "[",
    after: "](https://example.com)",
    placeholder: "link text",
  },
  {
    label: "Inline code",
    Icon: Code,
    before: "`",
    after: "`",
    placeholder: "code",
  },
];
const lineFormats = [
  {
    label: "Heading",
    Icon: Heading2,
    prefix: () => "## ",
    placeholder: "Heading",
  },
  {
    label: "Bulleted list",
    Icon: List,
    prefix: () => "- ",
    placeholder: "List item",
  },
  {
    label: "Numbered list",
    Icon: ListOrdered,
    prefix: (index: number) => `${index + 1}. `,
    placeholder: "List item",
  },
  { label: "Quote", Icon: Quote, prefix: () => "> ", placeholder: "Quote" },
];

export function MarkdownEditor({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const id = useId();
  const input = useRef<HTMLTextAreaElement>(null);

  function replace(
    start: number,
    end: number,
    inserted: string,
    selectionStart: number,
    selectionEnd: number,
  ) {
    onChange(value.slice(0, start) + inserted + value.slice(end));
    requestAnimationFrame(() => {
      input.current?.focus();
      input.current?.setSelectionRange(selectionStart, selectionEnd);
    });
  }

  return (
    <div className="grid gap-2">
      <label htmlFor={id} className="text-sm font-medium">
        Findings (Markdown)
      </label>
      <div className="overflow-hidden rounded-md border border-input focus-within:border-ring">
        <div
          role="group"
          aria-label="Markdown formatting"
          className="flex flex-wrap gap-1 border-b bg-muted/50 p-1"
        >
          {lineFormats.map(({ label, Icon, prefix, placeholder }) => (
            <Button
              key={label}
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              title={label}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                const { selectionStart, selectionEnd } = input.current!;
                const start = selectionStart
                  ? value.lastIndexOf("\n", selectionStart - 1) + 1
                  : 0;
                const newline = value.indexOf(
                  "\n",
                  Math.max(selectionStart, selectionEnd - 1),
                );
                const end = newline < 0 ? value.length : newline;
                const lines = (value.slice(start, end) || placeholder).split(
                  "\n",
                );
                const inserted = lines
                  .map((line, index) => prefix(index) + line)
                  .join("\n");
                replace(
                  start,
                  end,
                  inserted,
                  start + prefix(0).length,
                  start + inserted.length,
                );
              }}
            >
              <Icon />
            </Button>
          ))}
          <span className="mx-0.5 self-stretch border-l" aria-hidden="true" />
          {inlineFormats.map(({ label, Icon, before, after, placeholder }) => (
            <Button
              key={label}
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              title={label}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                const { selectionStart: start, selectionEnd: end } =
                  input.current!;
                const selected = value.slice(start, end) || placeholder;
                const textStart = start + before.length;
                replace(
                  start,
                  end,
                  before + selected + after,
                  textStart,
                  textStart + selected.length,
                );
              }}
            >
              <Icon />
            </Button>
          ))}
        </div>
        <Textarea
          id={id}
          ref={input}
          rows={15}
          value={value}
          className="min-h-64 resize-y rounded-none border-0 px-3 py-3 font-mono text-sm leading-6 shadow-none focus-visible:ring-0"
          onChange={(event) => onChange(event.target.value)}
        />
      </div>
    </div>
  );
}
