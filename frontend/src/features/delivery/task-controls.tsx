import { useState, useRef, type ReactNode } from "react";
import { Popover } from "radix-ui";
import { Check, ChevronDown, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { BoardCard } from "./board-api";

export const taskReference = (card: BoardCard) =>
  card.number ? `#${card.number}` : `#${card.id.slice(-7)}`;
export function PriorityPill({ priority = "normal" }: { priority?: string }) {
  return (
    <span className={`task-pill priority-${priority}`}>
      <span aria-hidden="true">
        {priority === "high" ? "↑" : priority === "low" ? "↓" : "−"}
      </span>
      {priority === "high" ? "High" : priority === "low" ? "Low" : "Normal"}
    </span>
  );
}
export function SearchPicker({
  label,
  value,
  options,
  onSelect,
  disabled,
  placeholder,
  icon,
}: {
  label: string;
  value: string;
  options: { value: string; label: string; detail?: string }[];
  onSelect: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  icon?: ReactNode;
}) {
  const [open, setOpen] = useState(false),
    [search, setSearch] = useState("");
  const list = useRef<HTMLDivElement>(null);
  const matches = options.filter((o) =>
    `${o.label} ${o.detail || ""}`.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <Popover.Root
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        setSearch("");
      }}
    >
      <Popover.Trigger asChild>
        <Button
          variant={icon ? "ghost" : "outline"}
          size={icon ? "sm" : "default"}
          className={icon ? undefined : "task-picker-trigger"}
          role="combobox"
          aria-label={label}
          aria-expanded={open}
          disabled={disabled}
        >
          {icon}
          <span>{value}</span>
          {!icon && <ChevronDown size={14} />}
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className="task-picker" align="start" sideOffset={6}>
          <div className="task-picker-search">
            <Search size={15} />
            <Input
              autoFocus
              aria-label={`Search ${label.toLowerCase()}`}
              placeholder={placeholder || "Search…"}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  list.current
                    ?.querySelector<HTMLButtonElement>("button")
                    ?.focus();
                }
              }}
            />
          </div>
          <div
            ref={list}
            role="listbox"
            aria-label={label}
            className="task-picker-options"
            onKeyDown={(e) => {
              if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
              e.preventDefault();
              const buttons = Array.from(
                list.current?.querySelectorAll("button") || [],
              );
              const i = buttons.indexOf(
                document.activeElement as HTMLButtonElement,
              );
              buttons[
                (i + (e.key === "ArrowDown" ? 1 : -1) + buttons.length) %
                  buttons.length
              ]?.focus();
            }}
          >
            {matches.map((o) => (
              <button
                type="button"
                role="option"
                aria-selected={value === o.label}
                key={o.value}
                onClick={() => {
                  onSelect(o.value);
                  setOpen(false);
                }}
              >
                <span>
                  <strong>{o.label}</strong>
                  {o.detail && <small>{o.detail}</small>}
                </span>
                {value === o.label && <Check size={14} />}
              </button>
            ))}
            {!matches.length && (
              <p className="task-picker-empty">No matches found.</p>
            )}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
