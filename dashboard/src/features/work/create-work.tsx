import { AgentSelection } from "./agent-selection";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect } from "@/components/ui/native-select";
import { ErrorNotice, Form, Modal, useRequestKey } from "@/shared/components";
import { keys } from "@/shared/query-keys";
import { workspaceOptions } from "@/features/workspaces/api";
import { workApi, workKeys } from "./api";
import { workProfiles, type WorkProfile } from "./profiles";
export function CreateWork({
  project,
  close,
  created,
}: {
  project: string;
  close: () => void;
  created: (id: string) => void;
}) {
  const workspaces = useQuery(workspaceOptions(project));
  const [provider, setProvider] = useState("");
  const [model, setModel] = useState("");
  const [reasoning, setReasoning] = useState("");
  const [workspace, setWorkspace] = useState("");
  const [mode, setMode] = useState("inline");
  const [instruction, setInstruction] = useState("");
  const [path, setPath] = useState("");
  const [digest, setDigest] = useState("");
  const [artifacts, setArtifacts] = useState("");
  const [kind, setKind] = useState<WorkProfile>("software");
  const instructionTooLong =
    new TextEncoder().encode(instruction).length > 65536;
  const outputPaths = artifacts
    .split(/\r?\n/)
    .map((path) => path.trim())
    .filter(Boolean);
  const tooManyOutputs = outputPaths.length > 32;
  const key = useRequestKey(),
    client = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        project_id: project,
        workspace_id: workspace,
        ...(provider ? { provider } : {}),
        ...(provider && model ? { model } : {}),
        ...(reasoning ? { reasoning_effort: reasoning } : {}),
        ...(mode === "inline"
          ? { instruction }
          : {
              specification_path: path.trim(),
              specification_digest: digest.trim(),
            }),
        kind,
        artifact_paths: outputPaths,
      };
      return workApi.create(payload, key(payload));
    },
    onSuccess: async (item) => {
      await Promise.all([
        client.invalidateQueries({ queryKey: workKeys.list(project) }),
        client.invalidateQueries({ queryKey: keys.workspaces(project) }),
      ]);
      created(item.id);
    },
  });
  const canCreate =
    !mutation.isPending &&
    !tooManyOutputs &&
    (mode === "inline"
      ? !!instruction.trim() && !instructionTooLong
      : !!workspace && !!path.trim() && !!digest.trim());
  return (
    <Modal
      title="Create work item"
      onClose={() => {
        if (!mutation.isPending) close();
      }}
      closeOnEscape={!mutation.isPending}
      closeDisabled={mutation.isPending}
    >
      <Form
        onSubmit={() => {
          if (canCreate) mutation.mutate();
        }}
      >
        <fieldset disabled={mutation.isPending} className="space-y-5">
          <p className="form-note">
            Save a private instruction or reference an existing file. Running
            work is a separate action.
          </p>
          {workspaces.isError && (
            <ErrorNotice
              error={workspaces.error}
              onRetry={() => void workspaces.refetch()}
              retrying={workspaces.isFetching}
            />
          )}
          <label>
            Input mode
            <NativeSelect
              value={mode}
              onChange={(e) => setMode(e.target.value)}
            >
              <option value="inline">Write instruction</option>
              <option value="file">Existing file</option>
            </NativeSelect>
          </label>
          {mode === "inline" && (
            <div className="space-y-2">
              <label htmlFor="draft-instruction">Instruction</label>
              <Textarea
                id="draft-instruction"
                required
                aria-invalid={instructionTooLong || undefined}
                aria-describedby={
                  instructionTooLong ? "draft-instruction-error" : undefined
                }
                value={instruction}
                onChange={(e) => setInstruction(e.target.value)}
              />
              {instructionTooLong && (
                <span
                  id="draft-instruction-error"
                  className="block text-sm text-destructive"
                  role="alert"
                >
                  This instruction exceeds 64 KiB. Shorten it or choose Existing
                  file to reference a saved instruction.
                </span>
              )}
            </div>
          )}
          <AgentSelection
            workspace={workspace}
            provider={provider}
            model={model}
            reasoning={reasoning}
            onReasoning={(effort, nextProvider, nextModel) => {
              setReasoning(effort);
              setProvider(nextProvider);
              setModel(nextModel);
            }}
            disabled={mutation.isPending}
          />
          <label>
            Worker
            <NativeSelect
              required={mode === "file"}
              value={workspace}
              onChange={(e) => setWorkspace(e.target.value)}
            >
              <option value="">Automatic worker</option>
              {workspaces.data?.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name}
                </option>
              ))}
            </NativeSelect>
          </label>
          <label>
            Work profile
            <NativeSelect
              value={kind}
              onChange={(e) => setKind(e.target.value as WorkProfile)}
            >
              {workProfiles.map(({ kind, label }) => (
                <option key={kind} value={kind}>
                  {label}
                </option>
              ))}
            </NativeSelect>
          </label>
          {mode === "file" && (
            <>
              <label>
                Specification path
                <Input
                  required
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  placeholder="specifications/gallery.md"
                />
              </label>
              <label>
                Specification digest
                <Input
                  required
                  value={digest}
                  onChange={(e) => setDigest(e.target.value)}
                />
              </label>
            </>
          )}
          <div className="space-y-2">
            <label htmlFor="draft-outputs">Retained output paths</label>
            <Textarea
              id="draft-outputs"
              aria-invalid={tooManyOutputs || undefined}
              aria-describedby={
                tooManyOutputs
                  ? "draft-outputs-help draft-outputs-error"
                  : "draft-outputs-help"
              }
              value={artifacts}
              onChange={(e) => setArtifacts(e.target.value)}
              placeholder="outputs/report.json"
            />
            <p id="draft-outputs-help" className="form-note">
              One worker file per line, up to 32. Outputs are published for each
              attempt.
            </p>
            {tooManyOutputs && (
              <span
                id="draft-outputs-error"
                className="block text-sm text-destructive"
                role="alert"
              >
                Keep at most 32 output paths. There are {outputPaths.length}{" "}
                paths entered.
              </span>
            )}
          </div>
          {mutation.isError && <ErrorNotice error={mutation.error} />}
          <div className="form-actions">
            <Button type="button" variant="outline" onClick={close}>
              Cancel
            </Button>
            <Button disabled={!canCreate}>
              {mutation.isPending ? "Creating draft…" : "Create draft"}
            </Button>
          </div>
        </fieldset>
      </Form>
    </Modal>
  );
}
