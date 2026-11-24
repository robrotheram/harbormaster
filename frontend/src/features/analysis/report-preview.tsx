import {
  createContext,
  isValidElement,
  useContext,
  useMemo,
  useEffect,
  useRef,
  useState,
  type ReactElement,
} from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { ChevronDown, FileDown, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ErrorNotice } from "@/shared/components";
import { visualizationReference } from "./api";
import { downloadReport, reportHTML } from "./report-export";
import {
  reportFilename,
  reportReference,
  type ReportDocument,
  type ReportPart,
} from "./report-model";
import "./report-document.css";

function ReportVisual({ part }: { part: ReportPart }) {
  const { table, figure, item, note } = part;
  if (table) {
    return (
      <>
        {table.rows.length ? (
          <table>
            <caption>{item.title}</caption>
            <thead>
              <tr>
                {table.columns.map(({ name }) => (
                  <th scope="col" key={name}>
                    {name}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {table.rows.map((row, index) => (
                <tr key={index}>
                  {table.columns.map(({ name }, column) => (
                    <td key={name}>{String(row[column] ?? "Null")}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p>No rows are available.</p>
        )}
        {note && <p>{note}</p>}
      </>
    );
  }
  if (!figure) return <p>{note || "Visualization unavailable."}</p>;
  return (
    <figure>
      <img src={figure.src} alt={item.title} />
      {!!(
        figure.legend?.length ||
        figure.attribution ||
        figure.density ||
        note
      ) && (
        <figcaption>
          {!!figure.legend?.length && (
            <ul className="analysis-report-legend">
              {figure.legend.map(({ label, color }) => (
                <li key={label + color}>
                  <span
                    aria-hidden="true"
                    className="analysis-report-swatch"
                    style={{ backgroundColor: color }}
                  />
                  {label}
                </li>
              ))}
            </ul>
          )}
          {figure.density && (
            <div className="analysis-report-density">
              Lower density <span aria-hidden="true" /> Higher density
            </div>
          )}
          {figure.attribution && <div>{figure.attribution}</div>}
          {note && <div>{note}</div>}
        </figcaption>
      )}
    </figure>
  );
}

const ReportEmbeds = createContext<{
  worker: string;
  preparing: boolean;
  parts: Map<string, ReportPart>;
}>({ worker: "", preparing: false, parts: new Map() });

function ReportCode({ children }: { children?: React.ReactNode }) {
  const { worker, preparing, parts } = useContext(ReportEmbeds);
  const code = isValidElement(children)
    ? (children as ReactElement<{ className?: string; children?: string }>)
    : undefined;
  if (code?.props.className !== "language-harbormaster-visualization")
    return <pre>{children}</pre>;
  const reference = visualizationReference(
    String(code.props.children || ""),
    worker,
  );
  const part = reference && parts.get(reportReference(reference));
  if (part) return <ReportVisual part={part} />;
  return (
    <p role="status">
      {preparing && reference
        ? "Preparing visualization…"
        : "Visualization unavailable."}
    </p>
  );
}

const markdownComponents: NonNullable<
  Parameters<typeof Markdown>[0]["components"]
> = {
  img: ({ alt }) => (alt ? <span>{alt}</span> : null),
  pre: ReportCode,
};

export function ReportPreview({
  report,
  source,
  preparing = false,
  onEdit,
}: {
  report: ReportDocument;
  source: { path: string; worker: string };
  preparing?: boolean;
  onEdit?: () => void;
}) {
  const article = useRef<HTMLElement>(null);
  const [error, setError] = useState<unknown>();
  const [exporting, setExporting] = useState(false);
  const filename = reportFilename(report.title);
  const embeds = useMemo(
    () => ({
      worker: source.worker,
      preparing,
      parts: new Map(
        report.parts.map((part) => [reportReference(part.item), part]),
      ),
    }),
    [source.worker, preparing, report.parts],
  );
  useEffect(() => {
    article.current?.focus({ preventScroll: true });
  }, [report.title, source.path]);

  async function exportPDF() {
    if (!article.current || exporting) return;
    const content = article.current;
    setError(undefined);
    setExporting(true);
    try {
      const { downloadReportPDF } = await import("./report-pdf");
      await downloadReportPDF(report.title, content, filename + ".pdf");
    } catch (cause) {
      setError(cause);
    } finally {
      setExporting(false);
    }
  }

  return (
    <div className="min-w-0 px-4 py-5 sm:px-6">
      <header className="mb-4 flex justify-end">
        <div className="flex items-center gap-2">
          {onEdit && (
            <Button type="button" size="sm" variant="outline" onClick={onEdit}>
              <Pencil /> Edit
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" size="sm" disabled={exporting}>
                <FileDown /> {exporting ? "Exporting…" : "Export"}{" "}
                <ChevronDown />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                disabled={preparing}
                onSelect={() => void exportPDF()}
              >
                Download PDF
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={preparing}
                onSelect={() => {
                  if (article.current)
                    downloadReport(
                      filename + ".html",
                      reportHTML(report.title, article.current),
                      "text/html;charset=utf-8",
                    );
                }}
              >
                Download HTML
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() =>
                  downloadReport(
                    filename + ".md",
                    report.markdown,
                    "text/markdown;charset=utf-8",
                  )
                }
              >
                Download Markdown
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>
      {error !== undefined && (
        <ErrorNotice error={error} message="Could not export the PDF." />
      )}
      {report.parts.some((part) => part.unavailable) && (
        <p role="status" className="mb-3 text-sm text-muted-foreground">
          The report marks any results it could not load.
        </p>
      )}
      <div className="bg-muted/40 p-2 sm:p-5">
        <article
          aria-label="Report content"
          tabIndex={-1}
          ref={article}
          className="analysis-report-document border border-black/10"
        >
          <ReportEmbeds.Provider value={embeds}>
            <Markdown
              skipHtml
              remarkPlugins={[remarkGfm]}
              components={markdownComponents}
            >
              {report.markdown}
            </Markdown>
          </ReportEmbeds.Provider>
        </article>
      </div>
    </div>
  );
}
