import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Modal, ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import type { Workspace } from "@/shared/types";

type Subscription = {
  saved: boolean;
  account_id?: string;
  expires_at?: number;
  expired?: boolean;
  workers: string[];
};
export function SubscriptionButton({ workspace }: { workspace: Workspace }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <KeyRound className="size-3.5" />
        Subscription
      </Button>
      {open && (
        <SubscriptionDialog
          workspace={workspace}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
function SubscriptionDialog({
  workspace,
  onClose,
}: {
  workspace: Workspace;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["my-subscription"],
    queryFn: ({ signal }) =>
      request<Subscription>("/subscription", undefined, signal),
    retry: false,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState("");
  const saved = query.data;
  const loaded = saved?.workers.includes(workspace.id);
  async function action(path: string, body: unknown, message: string) {
    setBusy(true);
    setError(undefined);
    setMessage("");
    try {
      await request(path, body);
      await client.invalidateQueries({ queryKey: ["my-subscription"] });
      setMessage(message);
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  async function importFile(file?: File) {
    if (!file || busy) return;
    setError(undefined);
    if (file.size > 64 * 1024) {
      setError(new Error("Choose a Codex auth.json file smaller than 64 KiB."));
      return;
    }
    try {
      const auth = JSON.parse(await file.text());
      // Never send refresh tokens or API keys from the login file.
      if (
        auth.auth_mode !== "chatgpt" ||
        auth.OPENAI_API_KEY ||
        !auth.tokens?.access_token ||
        !auth.tokens?.account_id
      )
        throw new Error(
          "Choose a Codex auth.json created by signing in with ChatGPT.",
        );
      await action(
        "/subscription",
        {
          auth: {
            auth_mode: "chatgpt",
            tokens: {
              access_token: auth.tokens.access_token,
              account_id: auth.tokens.account_id,
            },
          },
        },
        "Your subscription was saved.",
      );
    } catch (error) {
      setError(error);
    }
  }
  return (
    <Modal
      title="Your Codex subscription"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <p className="form-note">
        Save your ChatGPT subscription once, then load it into a worker. Saved
        credentials are private to your signed-in account and encrypted on the
        server.
      </p>
      {query.isPending && <p role="status">Loading subscription…</p>}
      {(error || query.error) && <ErrorNotice error={error || query.error} />}
      {saved?.saved && (
        <div className="rounded border p-3 text-sm space-y-2">
          <p>Account: {saved.account_id}</p>
          <p>
            {saved.expired ? "Expired" : "Expires"}:{" "}
            {saved.expires_at &&
              new Date(saved.expires_at * 1000).toLocaleString()}
          </p>
          <p>
            {loaded
              ? "Loaded into this worker."
              : "Not loaded into this worker."}
          </p>
        </div>
      )}
      {saved && !saved.saved && <p>No subscription saved yet.</p>}
      <label className="grid gap-2 text-sm">
        {saved?.saved ? "Replace saved login" : "Import Codex login"}
        <Input
          type="file"
          accept=".json,application/json"
          disabled={busy || query.isPending || !!query.error}
          onChange={(event) => {
            const file = event.currentTarget.files?.[0];
            event.currentTarget.value = "";
            void importFile(file);
          }}
        />
      </label>
      <p className="form-note">
        Sign in with ChatGPT using Codex on your computer, then choose its{" "}
        <code>auth.json</code> from <code>~/.codex</code> (or your CODEX_HOME).
        Only the current access token is saved. Import a fresh login when it
        expires; refresh tokens and API keys are not saved.
      </p>
      <p className="form-note">
        Loading lets all work on{" "}
        <strong>{workspace.name || workspace.id}</strong> use your subscription
        until you unload or delete it. Other users cannot view your saved
        credentials or replace the loaded account. Unloading also ends active
        inference requests.
      </p>
      {message && <p role="status">{message}</p>}
      <div className="form-actions">
        <Button variant="outline" disabled={busy} onClick={onClose}>
          Close
        </Button>
        {saved?.saved && (
          <Button
            variant="destructive"
            disabled={busy}
            onClick={() =>
              action(
                "/subscription/delete",
                {},
                "Subscription deleted and unloaded from all workers.",
              )
            }
          >
            Delete saved subscription
          </Button>
        )}
        {loaded ? (
          <Button
            disabled={busy}
            onClick={() =>
              action(
                `/workspaces/${encodeURIComponent(workspace.id)}/subscription/unload`,
                {},
                "Subscription unloaded.",
              )
            }
          >
            Unload from worker
          </Button>
        ) : (
          <Button
            disabled={busy || !saved?.saved || saved.expired}
            onClick={() =>
              action(
                `/workspaces/${encodeURIComponent(workspace.id)}/subscription/load`,
                {},
                "Subscription loaded into worker.",
              )
            }
          >
            Load into worker
          </Button>
        )}
      </div>
    </Modal>
  );
}
