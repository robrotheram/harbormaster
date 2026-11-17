import { useQuery } from "@tanstack/react-query";
import { Popover } from "radix-ui";
import { SlidersHorizontal, ChevronDown, RefreshCw } from "lucide-react";
import { NativeSelect } from "@/components/ui/native-select";
import { Button } from "@/components/ui/button";
import { request } from "@/shared/http";
import { label } from "@/shared/format";

export type AgentModel = { provider: string; model: string };
type Catalog = {
  workspace_id: string;
  default_provider?: string;
  default_model?: string;
  items: AgentModel[];
};
export function AgentSelection({
  workspace,
  provider,
  model,
  onProvider,
  onModel,
  disabled = false,
  compact = false,
}: {
  workspace?: string;
  provider: string;
  model: string;
  onProvider: (value: string) => void;
  onModel: (value: string) => void;
  disabled?: boolean;
  compact?: boolean;
}) {
  const query = useQuery({
    queryKey: ["worker-models", workspace],
    queryFn: async ({ signal }) => {
      const data = await request<Catalog>(
        `/workspaces/${encodeURIComponent(workspace!)}/models`,
        undefined,
        signal,
      );
      if (data.workspace_id !== workspace || !Array.isArray(data.items))
        throw new Error("Model catalog unavailable");
      return data;
    },
    enabled: !!workspace,
    refetchInterval: 30000,
    retry: false,
  });
  const options = query.data?.items || [];
  const value = provider && model ? JSON.stringify([provider, model]) : "";
  const defaultAvailable = options.some(
    (s) =>
      s.provider === query.data?.default_provider &&
      s.model === query.data?.default_model,
  );
  const known = options.some(
    (s) => s.provider === provider && s.model === model,
  );
  const picker = (
    <div className="conversation-model-picker">
      <NativeSelect
        aria-label="Model"
        value={value}
        disabled={
          disabled ||
          !workspace ||
          query.isPending ||
          query.isError ||
          options.length === 0
        }
        title={
          query.isError
            ? "Model list unavailable. The worker default is still available; retry to choose a model."
            : "Model for this message"
        }
        onChange={(e) => {
          const [nextProvider, nextModel] = e.target.value
            ? (JSON.parse(e.target.value) as [string, string])
            : [
                query.data?.default_provider || "",
                query.data?.default_model || "",
              ];
          onProvider(nextProvider);
          onModel(nextModel);
        }}
      >
        <option value="" disabled={!!query.data && !defaultAvailable}>
          {query.isError
            ? "Worker default"
            : query.isPending && workspace
              ? "Loading models…"
              : query.data && options.length === 0
                ? "No models available"
                : query.data?.default_model
                  ? `${query.data.default_model} (worker default)`
                  : "Worker default"}
        </option>
        {value && !known && (
          <option value={value} disabled>
            {model} · {label(provider)}
          </option>
        )}
        {options.map((s) => (
          <option
            key={JSON.stringify([s.provider, s.model])}
            value={JSON.stringify([s.provider, s.model])}
          >
            {s.model} · {label(s.provider)}
          </option>
        ))}
      </NativeSelect>
      {query.isError && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Retry loading models"
          title="Retry loading models"
          onClick={() => void query.refetch()}
        >
          <RefreshCw size={14} />
        </Button>
      )}
    </div>
  );
  if (!compact) return picker;
  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="thread-model-trigger"
          aria-label={`Choose model: ${model || query.data?.default_model || "Model"}`}
          disabled={disabled}
        >
          <SlidersHorizontal size={14} />
          <span>{model || query.data?.default_model || "Model"}</span>
          <ChevronDown size={12} />
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          className="thread-model-popover"
          side="top"
          align="start"
          sideOffset={10}
        >
          <h3>Model for your next message</h3>
          {picker}
          <p>
            {query.isError
              ? "The model list is unavailable. You can continue with the configured model or retry loading the list."
              : "Changing the model applies to your next message. Earlier runs keep their original selection."}
          </p>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
