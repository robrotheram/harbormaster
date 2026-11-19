import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { Modal, ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";

export type AgentSubscription = {
  id: string;
  name: string;
  provider: "codex" | "claude";
  account_id?: string;
  expires_at: number;
  expired: boolean;
  workers: string[];
  default?: boolean;
};
export const subscriptionKey = ["my-subscriptions"];
export const providerNames = { codex: "ChatGPT / Codex", claude: "Claude" };
export function useSubscriptions() {
  return useQuery({
    queryKey: subscriptionKey,
    queryFn: ({ signal }) =>
      request<{ items: AgentSubscription[] }>(
        "/subscriptions",
        undefined,
        signal,
      ),
    retry: false,
  });
}

export function AgentSubscriptions() {
  const query = useSubscriptions();
  const client = useQueryClient();
  const [editing, setEditing] = useState<AgentSubscription | "new" | null>(
    null,
  );
  const [deleting, setDeleting] = useState<AgentSubscription>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [message, setMessage] = useState("");
  async function toggleDefault(item: AgentSubscription) {
    setBusy(true);
    setError(undefined);
    try {
      await request(`/subscriptions/${encodeURIComponent(item.id)}`, {
        default: !item.default,
      });
      await client.invalidateQueries({ queryKey: subscriptionKey });
      setMessage(
        item.default
          ? "Default removed."
          : `${item.name} is now your default for ${providerNames[item.provider]}.`,
      );
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!deleting) return;
    setBusy(true);
    setError(undefined);
    try {
      await request(
        `/subscriptions/${encodeURIComponent(deleting.id)}/delete`,
        {},
      );
      await client.invalidateQueries({ queryKey: subscriptionKey });
      setMessage(`${deleting.name} deleted and unloaded from its workers.`);
      setDeleting(undefined);
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-label="Agent subscriptions" className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <p className="text-sm text-muted-foreground">
            Your saved ChatGPT and Claude accounts. Only you can manage these
            subscriptions.
          </p>
        </div>
        <Button
          onClick={() => {
            setMessage("");
            setEditing("new");
          }}
        >
          <Plus className="size-4" />
          Add subscription
        </Button>
      </div>
      <p className="form-note">
        Choose a default for each provider to load it automatically when you
        send work to an unconfigured worker. Use Worker settings to load a
        different account.
      </p>
      {!!error && !deleting && <ErrorNotice error={error} />}
      {query.isPending && <p role="status">Loading subscriptions…</p>}
      {query.error && (
        <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />
      )}
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
      {query.data?.items.length === 0 && (
        <div className="rounded-md border border-dashed p-8 text-center space-y-2">
          <KeyRound className="mx-auto size-6 text-muted-foreground" />
          <h3 className="font-medium">No subscriptions saved</h3>
          <p className="text-sm text-muted-foreground">
            Add your first ChatGPT or Claude account to use it in a worker.
          </p>
        </div>
      )}
      <div className="grid gap-3">
        {query.data?.items.map((item) => (
          <article
            key={item.id}
            aria-label={item.name}
            className="rounded-md border bg-card p-4 flex flex-wrap items-center justify-between gap-4"
          >
            <div className="min-w-0 space-y-1">
              <h3 className="font-medium break-words">{item.name}</h3>
              <p className="text-sm text-muted-foreground">
                {providerNames[item.provider]}
                {item.account_id ? ` · ${item.account_id}` : ""}
              </p>
              <p className="text-sm">
                <span
                  className={
                    item.expired ? "text-destructive" : "text-muted-foreground"
                  }
                >
                  {item.expired
                    ? "Login expired — import a fresh login"
                    : `Expires ${new Date(item.expires_at * 1000).toLocaleString()}`}
                </span>
              </p>
              <p className="text-xs text-muted-foreground">
                {item.default && "Default · "}
                {item.workers.length
                  ? `Loaded into ${item.workers.length} worker${item.workers.length === 1 ? "" : "s"}`
                  : "Not loaded into any worker"}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={busy || (item.expired && !item.default)}
                onClick={() => void toggleDefault(item)}
              >
                {item.default ? "Remove default" : "Set as default"}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setMessage("");
                  setEditing(item);
                }}
              >
                Edit
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setError(undefined);
                  setDeleting(item);
                }}
              >
                Delete
              </Button>
            </div>
          </article>
        ))}
      </div>
      {editing && (
        <SubscriptionForm
          subscription={editing === "new" ? undefined : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            setMessage("Subscription saved.");
          }}
        />
      )}
      {deleting && (
        <Modal
          title="Delete subscription"
          onClose={() => {
            if (!busy) setDeleting(undefined);
          }}
        >
          <p>
            Delete <strong>{deleting.name}</strong>? It will be unloaded from
            all its workers, ending active inference requests using this
            account.
          </p>
          {!!error && <ErrorNotice error={error} />}
          <div className="form-actions">
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setDeleting(undefined)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={busy}
              onClick={() => void remove()}
            >
              Delete subscription
            </Button>
          </div>
        </Modal>
      )}
    </section>
  );
}

function SubscriptionForm({
  subscription,
  onClose,
  onSaved,
}: {
  subscription?: AgentSubscription;
  onClose: () => void;
  onSaved: () => void;
}) {
  const client = useQueryClient();
  const [name, setName] = useState(subscription?.name || "");
  const [provider, setProvider] = useState<"codex" | "claude">(
    subscription?.provider || "codex",
  );
  const [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  async function save(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      let auth: unknown;
      if (file) {
        if (file.size > 64 * 1024)
          throw new Error("Choose a login file smaller than 64 KiB.");
        const input = JSON.parse(await file.text());
        // Send only the current login snapshot, never refresh tokens or API keys.
        if (provider === "codex") {
          if (
            input.auth_mode !== "chatgpt" ||
            input.OPENAI_API_KEY ||
            !input.tokens?.access_token ||
            !input.tokens?.account_id
          )
            throw new Error(
              "Choose a Codex auth.json created by signing in with ChatGPT.",
            );
          auth = {
            auth_mode: "chatgpt",
            tokens: {
              access_token: input.tokens.access_token,
              account_id: input.tokens.account_id,
            },
          };
        } else {
          const oauth = input.claudeAiOauth;
          if (
            !oauth?.accessToken ||
            !oauth.expiresAt ||
            !Array.isArray(oauth.scopes)
          )
            throw new Error(
              "Choose a Claude Code .credentials.json created by signing in with Claude.",
            );
          auth = {
            claudeAiOauth: {
              accessToken: oauth.accessToken,
              expiresAt: oauth.expiresAt,
              scopes: oauth.scopes,
            },
          };
        }
      }
      await request(
        subscription
          ? `/subscriptions/${encodeURIComponent(subscription.id)}`
          : "/subscriptions",
        { name: name.trim(), provider, ...(auth ? { auth } : {}) },
      );
      setFile(undefined);
      await client.invalidateQueries({ queryKey: subscriptionKey });
      onSaved();
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title={subscription ? "Edit subscription" : "Add subscription"}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form onSubmit={save} className="space-y-4">
        {!!error && <ErrorNotice error={error} />}
        <label className="grid gap-2 text-sm">
          Name
          <Input
            autoFocus
            required
            maxLength={80}
            value={name}
            disabled={busy}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Personal or Work"
          />
        </label>
        <label className="grid gap-2 text-sm">
          Provider
          <NativeSelect
            aria-label="Provider"
            value={provider}
            disabled={busy || !!subscription}
            onChange={(e) => {
              setProvider(e.target.value as "codex" | "claude");
              setFile(undefined);
            }}
          >
            <option value="codex">ChatGPT / Codex</option>
            <option value="claude">Claude</option>
          </NativeSelect>
        </label>
        <label className="grid gap-2 text-sm">
          {subscription ? "Replace login (optional)" : "Login file"}
          <Input
            key={provider}
            type="file"
            accept=".json,application/json"
            required={!subscription}
            disabled={busy}
            onChange={(e) => setFile(e.target.files?.[0])}
          />
        </label>
        <p className="form-note">
          {provider === "codex" ? (
            <>
              Sign in with ChatGPT in Codex on your computer, then select{" "}
              <code>~/.codex/auth.json</code> (or the file in your CODEX_HOME).
            </>
          ) : (
            <>
              Sign in with your Claude subscription in Claude Code, then select{" "}
              <code>~/.claude/.credentials.json</code> where available. Some
              systems keep the login in a system keychain instead of a file.
            </>
          )}
        </p>
        <p className="form-note">
          Your current access token is encrypted on the server. Import a fresh
          login when it expires. Refresh tokens and API keys are not saved.
        </p>
        {subscription && (
          <p className="form-note">
            Replacing the login also updates workers using this subscription and
            ends active inference requests using the old login.
          </p>
        )}
        <div className="form-actions">
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            disabled={busy || !name.trim() || (!subscription && !file)}
          >
            {busy ? "Saving…" : "Save subscription"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
