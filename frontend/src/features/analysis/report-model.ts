import {
  visualizationReference,
  type ArtifactSpec,
  type DataPage,
  type VisualizationReference,
} from "./api";

export type ReportDraft = {
  title: string;
  findings?: string;
};

export type ReportRevision = { path: string; title: string; worker_id: string };

export type ReportFigure = {
  src: string;
  attribution?: string;
  legend?: { label: string; color: string }[];
  density?: boolean;
};

export type ReportSource = VisualizationReference & {
  title?: string;
  type?: ArtifactSpec["type"];
};

export type ReportPart = {
  item: ReportSource & { title: string };
  table?: DataPage;
  figure?: ReportFigure;
  note?: string;
  unavailable?: boolean;
};

export type ReportDocument = {
  title: string;
  markdown: string;
  parts: ReportPart[];
};

export const defaultReportDraft: ReportDraft = {
  title: "Analysis report",
};

const reportHeading = /^\s*# +(.+?) *(?: +#+)? *(?:\r?\n|$)/;

export function reportDraft(markdown: string, path: string): ReportDraft {
  const heading = reportHeading.exec(markdown);
  const name = path
    .split("/")
    .at(-1)
    ?.replace(/\.(?:md|markdown)$/i, "");
  return {
    ...defaultReportDraft,
    title:
      heading?.[1].replace(/\\([!#&()[\]*_<>~.\\|{}])/g, "$1") ||
      name ||
      defaultReportDraft.title,
    findings: markdown
      .slice(heading?.[0].length ?? 0)
      .replace(/^(?:\r?\n)+|(?:\r?\n)+$/g, ""),
  };
}

export function reportReference(item: VisualizationReference) {
  return `${item.worker_id}/${item.artifact_id}`;
}

export function reportReferences(markdown: string, worker: string) {
  const references = new Map<string, VisualizationReference>();
  let fence:
    | { marker: string; length: number; reference: boolean; body: string[] }
    | undefined;
  for (const line of markdown.split(/\r?\n/)) {
    const delimiter = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line);
    if (!fence && delimiter) {
      fence = {
        marker: delimiter[1][0],
        length: delimiter[1].length,
        reference: delimiter[2].trim() === "harbormaster-visualization",
        body: [],
      };
    } else if (
      fence &&
      delimiter &&
      delimiter[1][0] === fence.marker &&
      delimiter[1].length >= fence.length &&
      !delimiter[2].trim()
    ) {
      if (fence.reference) {
        const reference = visualizationReference(fence.body.join("\n"), worker);
        if (reference?.worker_id)
          references.set(reportReference(reference), reference);
      }
      fence = undefined;
    } else if (fence?.reference) fence.body.push(line);
  }
  return [...references.values()];
}

export function reportFindings(reply: string) {
  return reply
    .replaceAll("\r\n", "\n")
    .replace(
      /^ {0,3}```(?:harbormaster-visualization|workbench-chart)[^\n]*\n[\s\S]*?^ {0,3}```[ \t]*$/gm,
      "",
    )
    .trim();
}

export function reportSnippet(item: VisualizationReference) {
  return ["```harbormaster-visualization", reportReference(item), "```"].join(
    "\n",
  );
}

export function reportDataNote(page: DataPage, table: boolean) {
  const notes: string[] = [];
  if (page.sampled)
    notes.push(
      `Based on a sample of ${page.rows.length.toLocaleString()} rows.`,
    );
  if (table && page.has_more)
    notes.push(`Showing the first ${page.rows.length.toLocaleString()} rows.`);
  if (page.truncated)
    notes.push(
      "Some rows or values were clipped to fit the worker response limit.",
    );
  return notes.join(" ");
}

export function reportMarkdown(title: string, findings: string) {
  const heading = title
    .replace(/[\r\n\t]/g, " ")
    .replace(/[\\`*_\[\]{}<>()#!|~&]/g, "\\$&");
  return findings ? `# ${heading}\n\n${findings}\n` : `# ${heading}\n`;
}

export function reportFilename(title: string) {
  return (
    title
      .trim()
      .toLowerCase()
      .replace(/[^\p{Letter}\p{Number}]+/gu, "-")
      .slice(0, 90)
      .replace(/^-+|-+$/g, "") || "analysis-report"
  );
}
