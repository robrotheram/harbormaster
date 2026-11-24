import pdfMake from "pdfmake/build/pdfmake";
import fonts from "pdfmake/build/vfs_fonts";
import type {
  Content,
  ContentText,
  Style,
  TableCell,
  TDocumentDefinitions,
} from "pdfmake/interfaces";

const accent = "#235c64";
const muted = "#465768";
const svgPrefix = "data:image/svg+xml,";
const linkScheme = /^(https?:|mailto:)/i;

pdfMake.addVirtualFileSystem(fonts);

function inline(parent: Node, style: Partial<ContentText> = {}): ContentText[] {
  return Array.from(parent.childNodes).flatMap((node): ContentText[] => {
    if (node.nodeType === Node.TEXT_NODE) {
      return [{ ...style, text: node.textContent ?? "" }];
    }
    if (!(node instanceof HTMLElement)) return [];
    switch (node.tagName) {
      case "BR":
        return [{ text: "\n" }];
      case "STRONG":
        return inline(node, { ...style, bold: true });
      case "EM":
        return inline(node, { ...style, italics: true });
      case "DEL":
        return inline(node, { ...style, decoration: "lineThrough" });
      case "CODE":
        return inline(node, { ...style, color: accent, fontSize: 9 });
      case "A": {
        const href = node.getAttribute("href");
        return inline(node, {
          ...style,
          ...(href && linkScheme.test(href)
            ? { link: href, color: accent, decoration: "underline" }
            : {}),
        });
      }
      default:
        return inline(node, style);
    }
  });
}

function table(element: HTMLTableElement): Content[] {
  const rows = Array.from(element.rows);
  const columns = rows[0]?.cells.length ?? 0;
  const headerRows = element.tHead?.rows.length ?? 0;
  const blocks: Content[] = [];
  if (element.caption)
    blocks.push({ text: inline(element.caption), style: "caption" });
  for (let offset = 0; offset < columns; offset += 6) {
    const width = Math.min(6, columns - offset);
    const body = rows.map((row): TableCell[] => {
      return Array.from({ length: width }, (_, index) => {
        const cell = row.cells[offset + index];
        return {
          text: cell ? inline(cell) : "",
          bold: cell?.tagName === "TH",
          fillColor: cell?.tagName === "TH" ? "#edf3f4" : undefined,
        };
      });
    });
    blocks.push({
      table: { headerRows, widths: Array<string>(width).fill("*"), body },
      layout: {
        hLineWidth: () => 0.45,
        vLineWidth: () => 0,
        hLineColor: () => "#c7d2da",
        paddingTop: () => 5,
        paddingBottom: () => 5,
      },
      fontSize: 9,
      margin: [0, 2, 0, 10],
    });
  }
  return blocks;
}

function figure(element: HTMLElement): Content[] {
  const img = element.querySelector("img");
  const source = img?.getAttribute("src") ?? "";
  const size = {
    fit: [487, 310] as [number, number],
    alignment: "center" as const,
  };
  const image: Content | undefined = source.startsWith(svgPrefix)
    ? { svg: decodeURIComponent(source.slice(svgPrefix.length)), ...size }
    : source.startsWith("data:image/png;base64,")
      ? { image: source, ...size }
      : undefined;
  if (!image) return [];
  const stack: Content[] = [image];
  if (img?.alt) stack.push({ text: img.alt, style: "caption", bold: true });
  const caption = element.querySelector("figcaption");
  for (const part of Array.from(caption?.children ?? [])) {
    if (part.matches(".analysis-report-legend")) {
      const labels: ContentText[] = [];
      for (const item of Array.from(part.children)) {
        const swatch = item.querySelector<HTMLElement>(
          ".analysis-report-swatch",
        );
        const rgb = swatch?.style.backgroundColor.match(/\d+/g)?.slice(0, 3);
        const color =
          rgb?.length === 3
            ? "#" +
              rgb
                .map((channel) => Number(channel).toString(16).padStart(2, "0"))
                .join("")
            : accent;
        labels.push(
          { text: "● ", color },
          { text: (item.textContent ?? "") + "    " },
        );
      }
      stack.push({ text: labels, style: "caption" });
    } else {
      stack.push({ text: part.textContent ?? "", style: "caption" });
    }
  }
  return [{ stack, unbreakable: true, margin: [0, 4, 0, 12] }];
}

function blocks(parent: HTMLElement): Content[] {
  return Array.from(parent.children).flatMap((element): Content[] => {
    if (!(element instanceof HTMLElement)) return [];
    const tag = element.tagName;
    if (/^H[1-6]$/.test(tag)) {
      const level = Number(tag[1]);
      return [
        {
          text: inline(element),
          style: level === 1 ? "title" : level === 2 ? "section" : "subsection",
          headlineLevel: level,
          outline: true,
        },
      ];
    }
    if (element instanceof HTMLTableElement) return table(element);
    if (tag === "FIGURE") return figure(element);
    if (tag === "UL" || tag === "OL") {
      const items = Array.from(element.children).map((item): Content => {
        const stack: Content[] = [];
        let runs: ContentText[] = [];
        function flush() {
          if (runs.length) stack.push({ text: runs });
          runs = [];
        }
        for (const node of Array.from(item.childNodes)) {
          if (
            node instanceof HTMLElement &&
            /^(OL|UL|P|BLOCKQUOTE|PRE)$/.test(node.tagName)
          ) {
            flush();
            const holder = document.createElement("div");
            holder.append(node.cloneNode(true));
            stack.push(...blocks(holder));
          } else if (node.textContent?.trim()) {
            const holder = document.createElement("span");
            holder.append(node.cloneNode(true));
            runs.push(...inline(holder));
          } else if (node.nodeType === Node.TEXT_NODE && runs.length) {
            runs.push({ text: node.textContent ?? "" });
          }
        }
        flush();
        return { stack, margin: [0, 0, 0, 3] };
      });
      if (tag === "UL") return [{ ul: items, margin: [4, 0, 0, 7] }];
      return [
        {
          ol: items,
          start: (element as HTMLOListElement).start,
          margin: [4, 0, 0, 7],
        },
      ];
    }
    if (tag === "PRE")
      return [
        {
          text: element.textContent ?? "",
          fontSize: 9,
          color: muted,
          margin: [10, 2, 0, 9],
        },
      ];
    if (tag === "BLOCKQUOTE")
      return [
        {
          stack: blocks(element),
          italics: true,
          color: muted,
          margin: [12, 1, 0, 5],
        },
      ];
    if (tag === "HR")
      return [
        {
          canvas: [
            {
              type: "line",
              x1: 0,
              y1: 0,
              x2: 487,
              y2: 0,
              lineColor: "#c7d2da",
              lineWidth: 0.5,
            },
          ],
          margin: [0, 4, 0, 9],
        },
      ];
    if (tag === "SECTION") return blocks(element);
    return [{ text: inline(element), margin: [0, 0, 0, 8] }];
  });
}

export async function downloadReportPDF(
  title: string,
  article: HTMLElement,
  filename: string,
) {
  const heading: Style = { bold: true, color: "#172635" };
  const document: TDocumentDefinitions = {
    info: { title },
    pageSize: "A4",
    pageMargins: [54, 52, 54, 52],
    defaultStyle: {
      font: "Roboto",
      fontSize: 11,
      color: "#253241",
      lineHeight: 1.22,
    },
    styles: {
      title: { ...heading, fontSize: 23, margin: [0, 0, 0, 19] },
      section: { ...heading, fontSize: 15, margin: [0, 11, 0, 6] },
      subsection: { ...heading, fontSize: 12, margin: [0, 9, 0, 5] },
      caption: { color: muted, fontSize: 9, margin: [0, 4, 0, 0] },
    },
    footer: (page, count) => ({
      text: `${page} / ${count}`,
      alignment: "right",
      fontSize: 9,
      color: muted,
      margin: [54, 16, 54, 0],
    }),
    pageBreakBefore: (node, container) =>
      !!node.headlineLevel &&
      !container.getFollowingNodesOnPage().length &&
      !!container.getNodesOnNextPage().length,
    content: blocks(article),
  };
  await pdfMake.createPdf(document).download(filename);
}
