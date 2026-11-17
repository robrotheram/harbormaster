import { useEffect, useState, useRef, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { request } from "@/shared/http";
import { ErrorNotice } from "@/shared/components";
import { NativeSelect } from "@/components/ui/native-select";
import { Button } from "@/components/ui/button";
import { Settings2 } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { workKeys, type WorkItem } from "./api";
export type Roster = {
  items: { user_id: string; email: string; status: string; role: string }[];
  user_id: string;
  can_manage: boolean;
};
export const memberOptions = (project: string) => ({
  queryKey: ["project", project, "members"],
  queryFn: () =>
    request<Roster>(`/projects/${encodeURIComponent(project)}/members`),
});
export function Responsibility({
  item,
  children,
  running = false,
}: {
  item: WorkItem;
  children?: ReactNode;
  running?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const client = useQueryClient();
  const roster = useQuery(memberOptions(item.project_id));
  const [controller, setController] = useState(item.controller_id || "");
  const [reviewer, setReviewer] = useState(item.reviewer_id || "");
  useEffect(() => {
    setController(item.controller_id || "");
    setReviewer(item.reviewer_id || "");
  }, [item.controller_id, item.reviewer_id]);
  const change = useMutation({
    mutationFn: () =>
      request(`/work-items/${encodeURIComponent(item.id)}/responsibility`, {
        project_id: item.project_id,
        controller_id: controller,
        reviewer_id: reviewer,
        expected_controller_id: item.controller_id || "",
        expected_reviewer_id: item.reviewer_id || "",
        expected_attempt_id: item.latest_attempt_id || "",
      }),
    onSettled: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: workKeys.list(item.project_id) }),
        client.invalidateQueries({
          queryKey: workKeys.detail(item.project_id, item.id),
        }),
      ]),
  });
  const members = roster.data?.items.filter((m) => m.status === "active") || [];
  const name = (id?: string) =>
    members.find((m) => m.user_id === id)?.email ||
    (id ? "Former member" : "Unassigned");
  const canManage =
    roster.data &&
    (roster.data.can_manage ||
      roster.data.user_id === item.controller_id ||
      !item.controller_id);
  return (
    <>
      <Button
        ref={trigger}
        variant="ghost"
        size="sm"
        onClick={() => setOpen(true)}
        aria-label="Task settings"
      >
        <Settings2 size={16} /> <span>Settings</span>
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          className="thread-settings-sheet"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            trigger.current?.focus();
          }}
        >
          <SheetHeader>
            <SheetTitle>Task settings</SheetTitle>
            <SheetDescription>
              Manage agent control, review and the work environment.
            </SheetDescription>
          </SheetHeader>
          <div className="thread-settings-body">
            {children && (
              <section className="thread-environment">
                <h3>Environment</h3>
                {children}
              </section>
            )}
            <section className="thread-responsibility-form">
              <h3>Agent control</h3>
              <p className="thread-settings-hint">
                {running
                  ? "Stop the current run before changing control or the reviewer."
                  : "Choose who can direct this agent and who reviews its output."}
              </p>
              {!canManage && (
                <p>
                  Controller: {name(item.controller_id)} · Reviewer:{" "}
                  {name(item.reviewer_id)}
                </p>
              )}
              {roster.isError && <ErrorNotice error={roster.error} />}{" "}
              {change.isError && <ErrorNotice error={change.error} />}
              {canManage && (
                <>
                  <label>
                    Agent controller
                    <NativeSelect
                      aria-label="Agent controller"
                      value={controller}
                      disabled={running || change.isPending}
                      onChange={(e) => setController(e.target.value)}
                    >
                      <option value="">Unassigned</option>
                      {members.map((m) => (
                        <option key={m.user_id} value={m.user_id}>
                          {m.email}
                        </option>
                      ))}
                    </NativeSelect>
                  </label>
                  <label>
                    Required reviewer
                    <NativeSelect
                      aria-label="Required reviewer"
                      value={reviewer}
                      disabled={running || change.isPending}
                      onChange={(e) => setReviewer(e.target.value)}
                    >
                      <option value="">Any project member</option>
                      {members.map((m) => (
                        <option key={m.user_id} value={m.user_id}>
                          {m.email}
                        </option>
                      ))}
                    </NativeSelect>
                  </label>
                  <Button
                    size="sm"
                    disabled={
                      running ||
                      change.isPending ||
                      roster.isError ||
                      (controller === (item.controller_id || "") &&
                        reviewer === (item.reviewer_id || ""))
                    }
                    onClick={() => change.mutate()}
                  >
                    {change.isPending ? "Saving…" : "Save changes"}
                  </Button>
                </>
              )}
              {!!item.control_history?.length && (
                <ul>
                  {item.control_history.map((h, i) => (
                    <li key={i}>
                      {name(h.from)} → {name(h.to)} ·{" "}
                      {new Date(h.at).toLocaleString()}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </div>
        </SheetContent>
      </Sheet>
    </>
  );
}
