import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice } from "@/shared/components";
import { request } from "@/shared/http";

export type ForgeAccount = {
  name: string;
  provider: string;
  api_url: string;
  revision?: number;
  managed?: boolean;
};
export function AccountForm({
  account,
  onSaved,
  onCancel,
}: {
  account?: ForgeAccount;
  onSaved: (account: ForgeAccount) => Promise<void>;
  onCancel: () => void;
}) {
  const [name, setName] = useState(account?.name || "");
  const [provider, setProvider] = useState(
    account
      ? account.provider === "github"
        ? "github"
        : account.api_url === "https://gitlab.com/api/v4"
          ? "gitlab"
          : "self-hosted"
      : "github",
  );
  const [address, setAddress] = useState(account?.api_url || "");
  const [token, setToken] = useState("");
  const save = useMutation({
    mutationFn: () =>
      request<ForgeAccount>("/forge-hosts", {
        name: name.trim(),
        provider: provider === "github" ? "github" : "gitlab",
        api_url:
          provider === "github"
            ? "https://api.github.com"
            : provider === "gitlab"
              ? "https://gitlab.com/api/v4"
              : address.trim(),
        token,
        revision: account?.revision || 0,
      }),
    onSuccess: async (value) => {
      setToken("");
      await onSaved(value);
    },
  });
  return (
    <form
      className="space-y-4 rounded-md border p-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (
          !save.isPending &&
          name.trim() &&
          token.trim() &&
          (provider !== "self-hosted" || address.trim())
        )
          save.mutate();
      }}
    >
      <h2 className="font-semibold">
        {account ? `Update token / ${account.name}` : "Add provider account"}
      </h2>
      <fieldset disabled={save.isPending} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="space-y-1 text-sm">
            Account name
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              maxLength={80}
              disabled={!!account}
              placeholder="Engineering GitLab"
            />
          </label>
          <label className="space-y-1 text-sm">
            Provider
            <NativeSelect
              aria-label="Provider"
              value={provider}
              disabled={!!account}
              onChange={(e) => {
                setProvider(e.target.value);
                setToken("");
              }}
            >
              <option value="github">GitHub</option>
              <option value="gitlab">GitLab.com</option>
              <option value="self-hosted">GitLab self-hosted</option>
            </NativeSelect>
          </label>
        </div>
        {provider === "self-hosted" && (
          <label className="block space-y-1 text-sm">
            GitLab address
            <Input
              type="url"
              aria-label="GitLab address"
              aria-describedby="forge-address-help"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              disabled={!!account}
              required
              placeholder="https://gitlab.example.com"
            />
            <span
              id="forge-address-help"
              className="text-xs text-muted-foreground"
            >
              Use your HTTPS GitLab address. Installation subpaths are
              supported.
            </span>
          </label>
        )}
        <label className="block space-y-1 text-sm">
          Access token
          <Input
            type="password"
            aria-label="Access token"
            aria-describedby="forge-token-help"
            autoComplete="new-password"
            spellCheck={false}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            required
            maxLength={8192}
            placeholder="Paste your access token"
          />
          <span id="forge-token-help" className="text-xs text-muted-foreground">
            {provider === "github"
              ? "Use a personal access token with access to your repositories and Issues read/write permission."
              : "Use a personal, group or project access token with the api scope."}
          </span>
        </label>
        <p className="text-xs text-muted-foreground">
          The token is encrypted on the server and is never shown again. Saving
          checks account authentication; repository access is checked when you
          connect a repository.
        </p>
        {save.isError && <ErrorNotice error={save.error} />}
        <div className="flex gap-2">
          <Button
            disabled={
              !name.trim() ||
              !token.trim() ||
              (provider === "self-hosted" && !address.trim())
            }
          >
            {save.isPending ? "Testing connection…" : "Test & save account"}
          </Button>
          <Button variant="outline" type="button" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </fieldset>
    </form>
  );
}
