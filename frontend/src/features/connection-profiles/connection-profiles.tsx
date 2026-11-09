import { connectionProfilesApi } from "./api";
import { parseDestinations } from "./policy";
import { connectionProfilesKey, useConnectionProfiles } from "./api";
import {
  Plus,
  Network,
  Globe,
  LockKeyhole,
  Pencil,
  Search,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { date } from "@/shared/format";
import { NativeSelect } from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ErrorNotice, Form, Modal } from "@/shared/components";
import type { GlobalConnectionProfile, ConnectionPolicy } from "@/shared/types";

export function ConnectionProfiles() {
  const query = useConnectionProfiles();
  const client = useQueryClient();
  const [edit, setEdit] = useState<
    GlobalConnectionProfile | null | undefined
  >();
  const [search, setSearch] = useState("");
  const visible =
    query.data?.filter((p) =>
      (p.name + " " + p.description + " " + p.policy.destinations.join(" "))
        .toLowerCase()
        .includes(search.toLowerCase()),
    ) || [];
  return (
    <>
      <div className="page-title">
        <div>
          <div className="mb-2 flex items-center gap-2">
            <h1>Connection profiles</h1>
            <Badge variant="outline">Global</Badge>
          </div>
          <p>Define network access once. Reuse it across your projects.</p>
        </div>
        <Button onClick={() => setEdit(null)}>
          <Plus />
          Create profile
        </Button>
      </div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-xs leading-6 text-muted-foreground">
          Global edits apply on the next explicit workspace start or settings
          save. Running work keeps its applied policy; direct network changes
          require a stop/start.
        </p>
        <div className="relative">
          <Search className="absolute left-3 top-2.5 size-3.5 text-muted-foreground" />
          <Input
            aria-label="Search connection profiles"
            placeholder="Find a profile…"
            className="w-56 bg-background pl-9"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>
      {query.isError && <ErrorNotice error={query.error} />}
      {query.isPending ? (
        <Skeleton className="h-72 w-full" />
      ) : (
        <div className="overflow-hidden rounded-xl border bg-background">
          <Table
            aria-label="Global connection profiles"
            className="w-full md:min-w-[800px]"
          >
            <TableHeader className="max-md:hidden">
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="w-[30%] pl-5">Profile</TableHead>
                <TableHead>Access</TableHead>
                <TableHead>Destinations</TableHead>
                <TableHead>Version</TableHead>
                <TableHead>Updated</TableHead>
                <TableHead className="pr-5 text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className="max-md:block">
              {visible.map((p) => {
                const Icon =
                  p.policy.mode === "open"
                    ? Globe
                    : p.policy.mode === "blocked"
                      ? LockKeyhole
                      : Network;
                return (
                  <TableRow
                    key={p.id}
                    data-profile={p.id}
                    className="max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:gap-2 max-md:p-4"
                  >
                    <TableCell className="py-5 pl-5 whitespace-normal max-md:col-span-2 max-md:p-0">
                      <div className="flex items-start gap-3">
                        <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/30 text-muted-foreground">
                          <Icon className="size-4" />
                        </span>
                        <div>
                          <h2
                            className="max-w-64 truncate text-sm font-semibold"
                            title={p.name}
                          >
                            {p.name}
                          </h2>
                          <p className="mt-1 max-w-64 whitespace-normal break-words text-xs leading-5 text-muted-foreground">
                            {p.description || "Reusable connection policy"}
                          </p>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="max-md:col-span-2 max-md:p-0">
                      <Badge
                        variant="outline"
                        className={
                          p.policy.mode === "open"
                            ? "border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300"
                            : ""
                        }
                      >
                        {p.policy.mode === "open"
                          ? "Fully open"
                          : p.policy.mode === "blocked"
                            ? "Blocked"
                            : "Allowlist"}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-md:col-span-2 max-md:p-0">
                      <div className="max-w-60 space-y-1">
                        {p.policy.mode === "allowlist" ? (
                          <>
                            {p.policy.destinations.slice(0, 3).map((d) => (
                              <p
                                key={d}
                                className="truncate font-mono text-[11px] text-muted-foreground"
                                title={d}
                              >
                                {d}
                              </p>
                            ))}
                            {p.policy.destinations.length > 3 && (
                              <span className="text-[11px] text-muted-foreground">
                                +{p.policy.destinations.length - 3} more
                                destinations
                              </span>
                            )}
                            {!p.policy.destinations.length && (
                              <span className="text-xs text-muted-foreground">
                                None approved
                              </span>
                            )}
                          </>
                        ) : (
                          <span className="text-xs text-muted-foreground">
                            {p.policy.mode === "open"
                              ? "Direct networking and DNS"
                              : "Platform integrations only"}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground max-md:p-0">
                      v{p.revision}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground max-md:p-0">
                      {p.updated_at && !p.updated_at.startsWith("0001")
                        ? date(p.updated_at)
                        : "Built-in profile"}
                    </TableCell>
                    <TableCell className="pr-5 text-right max-md:col-span-2 max-md:p-0">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setEdit(p)}
                      >
                        <Pencil className="size-3.5" />
                        Edit profile
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })}
              {!visible.length && (
                <TableRow>
                  <TableCell
                    colSpan={6}
                    className="h-32 text-center text-muted-foreground"
                  >
                    No profiles match your search.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <div className="border-t px-5 py-3 text-xs text-muted-foreground">
            {visible.length} profiles · Available across all projects
          </div>
        </div>
      )}
      <div className="mt-5 flex items-start gap-2 text-xs leading-6 text-muted-foreground">
        <Network className="mt-1 size-3.5 shrink-0" />
        <p>
          Choose a profile when creating a workspace or in its Settings.
          Workspace-specific policies remain independent until you link a global
          profile.
        </p>
      </div>
      {edit !== undefined && (
        <ProfileEditor
          profile={edit}
          onClose={() => setEdit(undefined)}
          onSaved={() => {
            setEdit(undefined);
            client.invalidateQueries({ queryKey: connectionProfilesKey });
          }}
        />
      )}
    </>
  );
}
function ProfileEditor({
  profile,
  onClose,
  onSaved,
}: {
  profile: GlobalConnectionProfile | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(profile?.name || "");
  const [description, setDescription] = useState(profile?.description || "");
  const [mode, setMode] = useState<ConnectionPolicy["mode"]>(
    profile?.policy.mode || "allowlist",
  );
  const [entries, setEntries] = useState(
    profile?.policy.destinations.join("\n") || "",
  );
  const mutation = useMutation({
    mutationFn: () =>
      connectionProfilesApi.save(profile, {
        name: name.trim(),
        description: description.trim(),
        policy: {
          mode,
          destinations: mode === "allowlist" ? parseDestinations(entries) : [],
        },
      }),
    onSuccess: onSaved,
  });
  return (
    <Modal
      title={profile ? "Edit global profile" : "Create global profile"}
      onClose={onClose}
    >
      <Form onSubmit={() => mutation.mutate()}>
        <label>
          Profile name
          <Input
            required
            maxLength={80}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label>
          Description
          <Input
            maxLength={500}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label>
          Connection mode
          <NativeSelect
            aria-label="Connection mode"
            value={mode}
            onChange={(e) =>
              setMode(e.target.value as ConnectionPolicy["mode"])
            }
          >
            <option value="blocked">Blocked</option>
            <option value="allowlist">Allowlist</option>
            <option value="open">Fully open</option>
          </NativeSelect>
        </label>
        {mode === "allowlist" && (
          <>
            <label>
              Allowed destinations
              <Textarea
                aria-label="Allowed destinations"
                rows={7}
                value={entries}
                onChange={(e) => setEntries(e.target.value)}
                placeholder={"github.com:443\nregistry.npmjs.org:443"}
              />
            </label>
            <p className="form-note">
              One exact public hostname or IP and TCP port per line, maximum 64.
              HTTP/HTTPS proxy only; no wildcards or private/LAN targets.
            </p>
          </>
        )}
        {mode === "open" && (
          <p className="notice">
            Fully open allows Internet, LAN and reachable host services through
            direct networking and DNS. Project files and private agent homes
            remain separate.
          </p>
        )}
        <p className="form-note">
          This is a global profile. Changes apply to linked workspaces when they
          next start or save settings. They do not interrupt or silently change
          active work.
        </p>
        {mutation.isError && <ErrorNotice error={mutation.error} />}
        <div className="form-actions">
          <Button variant="outline" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="default" disabled={mutation.isPending}>
            {mutation.isPending ? "Saving…" : "Save profile"}
          </Button>
        </div>
      </Form>
    </Modal>
  );
}
