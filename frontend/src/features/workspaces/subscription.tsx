import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";
import type { Workspace } from "@/shared/types";
import {
  useSubscriptions,
  subscriptionKey,
  providerNames,
  type AgentSubscription,
} from "@/features/connections/subscriptions";

export function WorkerSubscriptions({ workspace }: { workspace: Workspace }) {
  const query = useSubscriptions();
  const providers = (
    Object.keys(providerNames) as (keyof typeof providerNames)[]
  ).filter(
    (provider) => workspace.agent_kind !== "hermes" || provider === "codex",
  );
  const [provider, setProvider] = useState<keyof typeof providerNames>(
    workspace.agent_kind === "hermes" ? "codex" : providers[0],
  );
  return (
    <div className="space-y-4">
      <section
        className="settings-panel"
        aria-labelledby="worker-subscription-title"
      >
        <h2 id="worker-subscription-title" className="settings-panel-title">
          Subscription account
        </h2>
        <p className="form-note">
          Choose an account for {workspace.name || workspace.id}. Your default
          is loaded when you run work for a provider with an empty slot.
        </p>
        <label className="grid gap-2 text-sm">
          Provider
          <NativeSelect
            aria-label="Subscription provider"
            value={provider}
            onChange={(event) =>
              setProvider(event.target.value as keyof typeof providerNames)
            }
          >
            {providers.map((value) => (
              <option key={value} value={value}>
                {providerNames[value]}
              </option>
            ))}
          </NativeSelect>
        </label>
      </section>
      {query.isPending && <p role="status">Loading subscriptions…</p>}
      {query.error && (
        <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />
      )}
      {query.data && (
        <ProviderSelection
          key={provider}
          provider={provider}
          workspace={workspace.id}
          items={query.data.items.filter((item) => item.provider === provider)}
        />
      )}
      <Button asChild variant="outline">
        <Link to="/connections" search={{ tab: "agents" }}>
          Manage saved accounts
        </Link>
      </Button>
    </div>
  );
}
function ProviderSelection({
  provider,
  workspace,
  items,
}: {
  provider: keyof typeof providerNames;
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
      await Promise.all([
        client.invalidateQueries({ queryKey: subscriptionKey }),
        client.invalidateQueries({ queryKey: ["worker-models", workspace] }),
      ]);
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
    <section aria-label={providerNames[provider]} className="settings-panel">
      <h2 className="settings-panel-title">{providerNames[provider]}</h2>
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
                {s.default ? " (default)" : ""}
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
