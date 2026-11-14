import {
  useEffect,
  useMemo,
  useState,
  isValidElement,
  type ReactElement,
} from "react";
import { useQuery } from "@tanstack/react-query";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { request } from "@/shared/http";
import { privateContentOptions } from "@/shared/private-content";
import { activeAttempt } from "./conversation-api";
import { evidenceURL, type Attempt } from "./api";

import { imagePath, chartPath, dataPath } from "./workspace-output";
import { ChartContent, WorkspaceChart } from "./chart-content";

const imageType = (path: string) => {
  const ext = path.split(".").at(-1)?.toLowerCase();
  return ext === "svg"
    ? "image/svg+xml"
    : ext === "jpg"
      ? "image/jpeg"
      : `image/${ext}`;
};

export function WorkspaceImage({
  src,
  alt,
  project,
  attempt,
}: {
  src: string;
  alt: string;
  project: string;
  attempt: Attempt;
}) {
  const path = imagePath(src);
  const retained = attempt.artifacts?.find(
    (a) => (a.source_path || a.name) === path,
  );
  const retainedURL = evidenceURL(retained?.content_url);
  const [url, setURL] = useState("");
  const [broken, setBroken] = useState(false);
  const query = useQuery({
    ...privateContentOptions,
    queryKey: [
      "project",
      project,
      "message-image",
      attempt.id,
      path,
      retainedURL,
    ],
    enabled: !!path && !activeAttempt(attempt.state),
    retry: false,
    queryFn: async ({ signal }) => {
      if (retainedURL?.startsWith("/ui-api/")) {
        const response = await fetch(retainedURL, {
          signal,
          cache: "no-store",
          redirect: "error",
        });
        if (!response.ok || !response.body)
          throw new Error("Image unavailable");
        const reader = response.body.getReader();
        const chunks: Uint8Array<ArrayBuffer>[] = [];
        let size = 0;
        try {
          for (;;) {
            const { value, done } = await reader.read();
            if (done) break;
            size += value.length;
            if (size > 1048576) throw new Error("Image too large");
            chunks.push(new Uint8Array(value));
          }
        } finally {
          await reader.cancel();
        }
        return new Blob(chunks, { type: imageType(path!) });
      }
      const result = await request<{ content: string }>(
        `/work-items/${attempt.work_item_id}/image`,
        { project_id: project, attempt_id: attempt.id, path },
        signal,
      );
      const binary = atob(result.content || "");
      if (binary.length > 1048576) throw new Error("Image too large");
      return new Blob([Uint8Array.from(binary, (c) => c.charCodeAt(0))], {
        type: imageType(path!),
      });
    },
  });
  useEffect(() => {
    setBroken(false);
    if (!query.data || query.isError) {
      setURL("");
      return;
    }
    const next = URL.createObjectURL(query.data);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [query.data, query.isError]);
  if (!path) return <span>{alt || "Image"} (unsupported image location)</span>;
  if (query.isError || broken)
    return (
      <span className="message-image-unavailable">
        {alt || path} — image unavailable.{" "}
        <button
          onClick={() => {
            setBroken(false);
            void query.refetch();
          }}
        >
          Retry
        </button>
      </span>
    );
  if (!url)
    return (
      <span className="conversation-muted">
        {activeAttempt(attempt.state)
          ? "Image will appear when the run finishes."
          : "Loading image…"}
      </span>
    );
  // SVG is an image resource, never injected into the DOM or run in an iframe.
  return (
    <span className="message-image">
      <img src={url} alt={alt || path} onError={() => setBroken(true)} />
      <a href={url} download={path.split("/").at(-1)}>
        Download image
      </a>
    </span>
  );
}

export function MessageContent({
  content,
  project,
  attempt,
  onAsk,
  embedData = false,
}: {
  content: string;
  project: string;
  attempt: Attempt;
  onAsk?: (message: string) => void;
  embedData?: boolean;
}) {
  const components = useMemo<Components>(
    () => ({
      p: ({ children }) => <div className="message-paragraph">{children}</div>,
      pre: ({ children }) => {
        const code = isValidElement(children)
          ? (children as ReactElement<{
              className?: string;
              children?: string;
            }>)
          : undefined;
        return code?.props.className === "language-workbench-chart" ? (
          <ChartContent
            source={String(code.props.children || "")}
            running={activeAttempt(attempt.state)}
            onAsk={onAsk}
          />
        ) : (
          <pre>{children}</pre>
        );
      },
      img: ({ src, alt }) => (
        <WorkspaceImage
          src={typeof src === "string" ? src : ""}
          alt={alt || ""}
          project={project}
          attempt={attempt}
        />
      ),
      a: ({ href, children }) =>
        href && /^https?:\/\//i.test(href) ? (
          <a href={href} target="_blank" rel="noopener noreferrer">
            {children}
          </a>
        ) : (embedData ? dataPath(href || "") : chartPath(href || "")) ? (
          <WorkspaceChart
            src={href!}
            title={typeof children === "string" ? children : "Explore data"}
            project={project}
            attempt={attempt}
            onAsk={onAsk}
          />
        ) : imagePath(href || "") ? (
          <WorkspaceImage
            src={href!}
            alt={typeof children === "string" ? children : "Generated image"}
            project={project}
            attempt={attempt}
          />
        ) : (
          <span>{children}</span>
        ),
    }),
    [
      project,
      onAsk,
      embedData,
      attempt.id,
      attempt.state,
      attempt.artifacts,
      attempt.work_item_id,
    ],
  );
  return (
    <div className="message-markdown">
      <Markdown remarkPlugins={[remarkGfm]} skipHtml components={components}>
        {content}
      </Markdown>
    </div>
  );
}
