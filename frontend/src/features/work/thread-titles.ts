import { useEffect, useRef, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { type WorkItem } from "./api";
import { conversationTitle, instructionOptions } from "./conversation-title";

const batchSize = 4;

// Request text stays private and is discarded when these observers unmount.
// A digest identifies immutable content. Separate title keys prevent ordinary
// work-list invalidation from rereading every private file during a search.
export function useThreadTitles(project: string, items: WorkItem[]) {
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
      enabled: index < count,
      retryOnMount: false,
    })),
  });
  const waiting = queries.slice(0, count).some((query) => query.isPending);
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
  const failed = queries.filter((query) => query.isError);
  const pending = queries.filter((query) => query.isPending).length;
  return {
    titles: new Map(
      items.map((item, index) => [
        item.id,
        conversationTitle(item, queries[index]?.data),
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
