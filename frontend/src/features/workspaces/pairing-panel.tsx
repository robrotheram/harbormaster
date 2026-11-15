import { useState } from "react";

import { date } from "@/shared/format";
import { Modal } from "@/shared/components";
import { Button } from "@/components/ui/button";

import { Textarea } from "@/components/ui/textarea";

import type { Workspace, Pairing } from "@/shared/types";
export function PairingPanel({
  workspace,
  value,
  onClose,
}: {
  workspace: Workspace;
  value: Pairing;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  return (
    <Modal title="Connect T3 desktop" onClose={onClose}>
      <p className="form-note">
        Connect your desktop to {workspace.name || "this worker"} on this
        computer. The same project files and conversations will appear.
      </p>
      <ol className="steps">
        <li>Copy the one-use pairing URL below.</li>
        <li>
          In T3 desktop, open{" "}
          <strong>Settings → Environments → Add environment</strong>.
        </li>
        <li>Paste the complete URL and confirm the workspace.</li>
      </ol>
      <label>
        Private pairing URL
        <Textarea
          className="pairing-url"
          rows={3}
          value={value.pairing_url}
          readOnly
          spellCheck={false}
          onFocus={(event) => event.target.select()}
        />
      </label>
      <p className="form-note">
        Expires {date(value.expires_at)}. Keep this link private.{" "}
        {value.pairing_url.startsWith("https:")
          ? "Your desktop must be able to reach this environment over HTTPS."
          : "It works on the same computer as Docker."}
      </p>
      {copyFailed && (
        <p role="alert" className="text-sm text-destructive">
          The browser could not copy the link. Select the pairing URL above and
          copy it manually.
        </p>
      )}
      <div className="form-actions">
        <Button variant="outline" onClick={onClose}>
          Close
        </Button>
        <a
          className="primary button"
          href={
            "t3code://pair?pairingUrl=" + encodeURIComponent(value.pairing_url)
          }
        >
          Open T3 desktop
        </a>
        <Button
          variant="default"
          onClick={async () => {
            setCopyFailed(false);
            try {
              await navigator.clipboard.writeText(value.pairing_url);
              setCopied(true);
            } catch {
              setCopied(false);
              setCopyFailed(true);
            }
          }}
        >
          {copied ? "Copied" : "Copy pairing URL"}
        </Button>
      </div>
    </Modal>
  );
}
