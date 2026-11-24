import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { request } from "@/shared/http";
import { ErrorNotice, Form } from "@/shared/components";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { keys } from "@/shared/query-keys";
import { useUserSession } from "@/features/auth/session";

type Member = {
  project_id: string;
  email: string;
  user_id: string;
  role: "admin" | "member";
  status: string;
  revision: number;
};
export function ProjectMembers({ project }: { project: string }) {
  const client = useQueryClient();
  const queryKey = ["project", project, "members"];
  const roster = useQuery({
    queryKey,
    queryFn: () =>
      request<{ items: Member[]; can_manage: boolean }>(
        `/projects/${encodeURIComponent(project)}/members`,
      ),
  });
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const save = useMutation({
    mutationFn: (value: {
      email: string;
      role: string;
      revision: number;
      action: string;
    }) => request(`/projects/${encodeURIComponent(project)}/members`, value),
    onError: () => client.invalidateQueries({ queryKey }),
    onSuccess: async () => {
      setEmail("");
      await client.invalidateQueries({ queryKey });
    },
  });
  return (
    <section className="settings-panel" aria-labelledby="members-title">
      <h2 id="members-title" className="settings-panel-title">
        Project members
      </h2>
      <p className="text-sm text-muted-foreground">
        Only explicitly invited members can access this project. Invitations
        appear after signing in with the invited email address; no email is
        sent.
      </p>
      {roster.isError && (
        <ErrorNotice
          error={roster.error}
          onRetry={() => void roster.refetch()}
        />
      )}
      {save.isError && <ErrorNotice error={save.error} />}
      {roster.isPending && <p role="status">Loading members…</p>}
      {roster.data?.can_manage && (
        <Form
          onSubmit={() => {
            const existing = roster.data.items.find(
              (m) => m.email === email.trim().toLowerCase(),
            );
            save.mutate({
              email: email.trim(),
              role,
              revision: existing?.revision || 0,
              action: "invite",
            });
          }}
        >
          <label>
            Email address
            <Input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </label>
          <label>
            Project role
            <NativeSelect
              value={role}
              onChange={(e) => setRole(e.target.value)}
            >
              <option value="member">Member</option>
              <option value="admin">Administrator</option>
            </NativeSelect>
          </label>
          <Button disabled={save.isPending || roster.isError}>
            Invite member
          </Button>
        </Form>
      )}
      <ul className="divide-y">
        {roster.data?.items
          .filter((m) => m.status !== "revoked")
          .map((m) => (
            <li
              key={m.email}
              className="flex flex-wrap items-center justify-between gap-3 py-3"
            >
              <div>
                {m.email}
                <p className="text-sm text-muted-foreground">
                  {m.role} · {m.status}
                </p>
              </div>
              {roster.data.can_manage && (
                <Button
                  variant="outline"
                  disabled={save.isPending || roster.isError}
                  onClick={() =>
                    save.mutate({
                      email: m.email,
                      role: m.role,
                      revision: m.revision,
                      action: "revoke",
                    })
                  }
                >
                  Revoke access for {m.email}
                </Button>
              )}
            </li>
          ))}
      </ul>
      {roster.data && !roster.data.items.length && (
        <p>No members yet. Invite the first project administrator.</p>
      )}
    </section>
  );
}
export function ProjectInvitations() {
  const session = useUserSession();
  const client = useQueryClient();
  const queryKey = ["project-invitations"];
  const invitations = useQuery({
    queryKey,
    enabled: session.status === "authenticated",
    queryFn: () => request<{ items: Member[] }>("/project-invitations"),
  });
  const accept = useMutation({
    mutationFn: (m: Member) =>
      request(
        `/projects/${encodeURIComponent(m.project_id)}/accept-invitation`,
        { revision: m.revision },
      ),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey }),
        client.invalidateQueries({ queryKey: keys.projects }),
      ]);
    },
  });
  if (session.status !== "authenticated") return null;
  return (
    <section className="space-y-3" aria-label="Project invitations">
      {invitations.isError && (
        <ErrorNotice
          error={invitations.error}
          onRetry={() => void invitations.refetch()}
        />
      )}
      {accept.isError && <ErrorNotice error={accept.error} />}{" "}
      {!!invitations.data?.items.length && <h2>Project invitations</h2>}
      {invitations.data?.items.map((m) => (
        <div
          key={m.project_id}
          className="flex flex-wrap items-center justify-between gap-3 rounded border p-4"
        >
          <span>
            {m.project_id} · Invited as {m.role}
          </span>
          <Button disabled={accept.isPending} onClick={() => accept.mutate(m)}>
            Accept invitation
          </Button>
        </div>
      ))}
    </section>
  );
}
