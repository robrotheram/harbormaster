for (const details of document.querySelectorAll("details[data-expand]")) {
  const desktop = window.matchMedia(details.dataset.expand);
  const update = () => {
    details.open = desktop.matches;
  };
  update();
  desktop.addEventListener("change", update);
}

const search = document.querySelector("[data-search-index]");
if (search) {
  const query = new URLSearchParams(window.location.search).get("q")?.trim() ?? "";
  document.querySelector("#site-query").value = query;
  if (query) {
    searchDocs(search.dataset.searchIndex, query);
  }
}

async function searchDocs(indexURL, query) {
  const status = document.querySelector("#search-status");
  const list = document.querySelector("#search-results");
  const terms = [...new Set(query.toLowerCase().split(/\s+/))];
  const includesTerms = (text) => terms.every((term) => text.toLowerCase().includes(term));
  status.textContent = "Searching the documentation…";

  try {
    const response = await fetch(indexURL);
    if (!response.ok) {
      throw new Error(`Search index returned ${response.status}`);
    }
    const pages = await response.json();
    const results = pages.flatMap((page) => {
      const heading = page.headings.find(({ title }) => includesTerms(title));
      const inTitle = includesTerms(page.title);
      if (!inTitle && !heading && !includesTerms(page.text)) {
        return [];
      }
      return [{ page, heading, rank: inTitle ? 3 : heading ? 2 : 1 }];
    });
    results.sort((a, b) => b.rank - a.rank || a.page.title.localeCompare(b.page.title));
    const count = Math.min(results.length, 25);
    status.textContent = count
      ? `Showing ${count} of ${results.length} ${results.length === 1 ? "result" : "results"} for “${query}”.`
      : `No results for “${query}”. Try fewer words or a guide title.`;

    for (const { page, heading, rank } of results.slice(0, count)) {
      const item = document.createElement("li");
      const link = document.createElement("a");
      link.href = rank === 2 ? `${page.url}#${heading.id}` : page.url;
      link.textContent = rank === 2 ? heading.title : page.title;
      item.append(link);
      const context = document.createElement("p");
      context.textContent = rank === 2 ? page.title : excerpt(page.text, terms);
      item.append(context);
      list.append(item);
    }
  } catch {
    status.textContent = "Search is unavailable. Use the documentation index or try again.";
  }
}

function excerpt(text, terms) {
  const lower = text.toLowerCase();
  const match = lower.indexOf(terms[0]);
  const start = Math.max(match - 34, 0);
  const end = start + 155;
  return `${start > 0 ? "…" : ""}${text.slice(start, end).trim()}${end < text.length ? "…" : ""}`;
}
