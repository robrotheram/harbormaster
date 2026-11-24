export type DataRow = Record<string, string | number>;

// A bounded CSV reader handles quoted commas, escaped quotes, and multiline cells.
export function parseVisualData(source: string): DataRow[] {
  if (source.length > 262144)
    throw new Error("Use a file smaller than 256 KB.");
  const text = source.trim();
  if (!text) return [];
  if (text.startsWith("[")) {
    const data: unknown = JSON.parse(text);
    if (
      !Array.isArray(data) ||
      data.some((r) => !r || typeof r !== "object" || Array.isArray(r))
    )
      throw new Error("Use a JSON array of objects, or CSV with a header row.");
    if (data.length > 2000) throw new Error("Use at most 2,000 rows.");
    return data.map((r) =>
      Object.fromEntries(
        Object.entries(r).filter(
          ([, v]) => typeof v === "string" || typeof v === "number",
        ),
      ),
    ) as DataRow[];
  }
  const rows: string[][] = [];
  let row: string[] = [],
    cell = "",
    quoted = false;
  for (let i = 0; i <= text.length; i++) {
    const char = text[i];
    if (char === '"') {
      if (quoted && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else quoted = !quoted;
    } else if (
      !quoted &&
      (char === "," || char === "\n" || char === undefined)
    ) {
      row.push(cell.replace(/\r$/, ""));
      cell = "";
      if (char !== ",") {
        rows.push(row);
        row = [];
      }
    } else if (char !== undefined) cell += char;
  }
  if (quoted)
    throw new Error("A quoted CSV cell is missing its closing quote.");
  const headers = rows.shift() || [];
  if (
    headers.length < 2 ||
    new Set(headers).size !== headers.length ||
    headers.some((h) => !h.trim())
  )
    throw new Error("CSV needs at least two distinct, named columns.");
  if (rows.length > 2000) throw new Error("Use at most 2,000 rows.");
  return rows
    .filter((r) => r.some(Boolean))
    .map((r) => Object.fromEntries(headers.map((h, i) => [h, r[i] || ""])));
}

export function numericColumns(rows: DataRow[]): string[] {
  return Object.keys(rows[0] || {}).filter((key) =>
    rows.every(
      (row) =>
        String(row[key] ?? "").trim() !== "" &&
        Number.isFinite(Number(row[key])),
    ),
  );
}
