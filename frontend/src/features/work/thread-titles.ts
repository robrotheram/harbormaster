import { boardOptions } from "@/features/delivery/board-api";
import { usePersonalWork } from "./personal-work";
import { useEffect, useRef, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { type WorkItem } from "./api";
import { conversationTitle, instructionOptions } from "./conversation-title";

const batchSize = 4;

// Request text stays private and is discarded when these observers unmount.
// A digest identifies immutable content. Separate title keys prevent ordinary
// work-list invalidation from rereading every private file during a search.
export function useThreadTitles(project: string, items: WorkItem[]) {
  const board = useQuery(boardOptions(project));
  const { titles: cache } = usePersonalWork();
  const stored = new Map(
    (board.data?.items || [])
      .filter((c) => c.title)
      .map((c) => [c.id, c.title!]),
  );
  const needsInstruction = (item: WorkItem) =>
    item.specification_path !== "" && !stored.has(item.id);
  const scope = JSON.stringify(
    items.map((item) => [item.id, item.specification_digest]),
  );
  const [batch, setBatch] = useState({ scope, count: batchSize });
  const count = batch.scope === scope ? batch.count : batchSize;
  const queries = useQueries({
    queries: items.map((item, index) => ({
      ...instructionOptions(project, item),
      queryKey: [
        "project",
        project,
        "thread-title",
        item.id,
        item.specification_digest,
      ],
      staleTime: "static" as const,
      enabled: index < count && needsInstruction(item),
      retryOnMount: false,
    })),
  });
  const waiting = queries
    .slice(0, count)
    .some((query, i) => query.isPending && needsInstruction(items[i]));
  useEffect(() => {
    if (!waiting && count < items.length)
      setBatch({ scope, count: count + batchSize });
  }, [waiting, count, items.length, scope]);

  const mounted = useRef(true);
  const currentScope = useRef(scope);
  currentScope.current = scope;
  const [retrying, setRetrying] = useState(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const failed = queries.filter(
    (query, i) => query.isError && needsInstruction(items[i]),
  );
  const pending = queries.filter(
    (query, i) => query.isPending && needsInstruction(items[i]),
  ).length;
  useEffect(() => {
    items.forEach((item, i) => {
      const title =
        stored.get(item.id) ||
        (queries[i]?.data
          ? conversationTitle(item, queries[i].data)
          : undefined);
      if (title) cache.current.set(item.id, title);
    });
  });
  return {
    owners: new Map(
      (board.data?.items || []).map((card) => [card.id, card.owner]),
    ),
    titles: new Map(
      items.map((item, index) => [
        item.id,
        stored.get(item.id) ||
          (queries[index]?.data
            ? conversationTitle(item, queries[index].data)
            : cache.current.get(item.id)) ||
          conversationTitle(item),
      ]),
    ),
    pending,
    failed: failed.length,
    retrying,
    retry: async () => {
      if (retrying) return;
      setRetrying(true);
      for (
        let i = 0;
        i < failed.length && mounted.current && currentScope.current === scope;
        i += batchSize
      )
        await Promise.all(
          failed.slice(i, i + batchSize).map((query) => query.refetch()),
        );
      if (mounted.current) setRetrying(false);
    },
  };
}
