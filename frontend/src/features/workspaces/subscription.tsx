import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { Modal, ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import type { Workspace } from "@/shared/types";
import {
  useSubscriptions,
  subscriptionKey,
  providerNames,
  type AgentSubscription,
} from "@/features/connections/subscriptions";

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
  const query = useSubscriptions();
  return (
    <Modal title="Worker subscriptions" onClose={onClose}>
      <p className="form-note">
        Choose a saved account for each provider. All work on{" "}
        <strong>{workspace.name || workspace.id}</strong> can use the loaded
        accounts until you unload them. Other users cannot replace your
        selections.
      </p>
      {query.isPending && <p role="status">Loading subscriptions…</p>}
      {query.error && <ErrorNotice error={query.error} />}
      {query.data &&
        (["codex", "claude"] as const).map((provider) => (
          <ProviderSelection
            key={provider}
            provider={provider}
            workspace={workspace.id}
            items={query.data.items.filter((s) => s.provider === provider)}
          />
        ))}
      <p className="form-note">
        Unloading ends active inference requests for that provider.
      </p>
      <div className="form-actions">
        <Button variant="outline" onClick={onClose}>
          Close
        </Button>
        <Button asChild variant="outline">
          <Link to="/connections" search={{ tab: "agents" }}>
            Manage subscriptions
          </Link>
        </Button>
      </div>
    </Modal>
  );
}
function ProviderSelection({
  provider,
  workspace,
  items,
}: {
  provider: "codex" | "claude";
  workspace: string;
  items: AgentSubscription[];
}) {
  const client = useQueryClient();
  const loaded = items.find((s) => s.workers.includes(workspace));
  const [selection, setSelection] = useState("");
  const selected = selection || loaded?.id || "";
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState("");
  async function action(unload: boolean) {
    setBusy(true);
    setError(undefined);
    setMessage("");
    try {
      await request(
        `/workspaces/${encodeURIComponent(workspace)}/subscription/${unload ? "unload" : "load"}`,
        { provider, subscription_id: selected },
      );
      await client.invalidateQueries({ queryKey: subscriptionKey });
      setSelection("");
      setMessage(
        unload ? "Subscription unloaded." : "Subscription loaded into worker.",
      );
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      aria-label={providerNames[provider]}
      className="rounded-md border p-4 space-y-3"
    >
      <h3 className="font-medium">{providerNames[provider]}</h3>
      <p className="text-sm text-muted-foreground">
        {loaded
          ? `Loaded: ${loaded.name}${loaded.expired ? " (login expired)" : ""}`
          : "None of your accounts are loaded for this provider."}
      </p>
      {items.length === 0 ? (
        <p className="text-sm">No saved accounts. Add one in Connections.</p>
      ) : (
        <label className="grid gap-2 text-sm">
          Saved account
          <NativeSelect
            aria-label="Saved account"
            value={selected}
            disabled={busy}
            onChange={(e) => setSelection(e.target.value)}
          >
            <option value="" disabled>
              Choose an account
            </option>
            {items.map((s) => (
              <option key={s.id} value={s.id} disabled={s.expired}>
                {s.name}
                {s.expired ? " (expired)" : ""}
              </option>
            ))}
          </NativeSelect>
        </label>
      )}
      {!!error && <ErrorNotice error={error} />}
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
      <div className="flex gap-2">
        {loaded && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => void action(true)}
          >
            Unload
          </Button>
        )}
        <Button
          disabled={
            busy ||
            !selected ||
            selected === loaded?.id ||
            !items.some((s) => s.id === selected && !s.expired)
          }
          onClick={() => void action(false)}
        >
          {loaded ? "Switch account" : "Load into worker"}
        </Button>
      </div>
    </section>
  );
}
