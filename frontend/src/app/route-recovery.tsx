import { useEffect, useRef } from "react";
import { AlertCircle, Search } from "lucide-react";
import { Button } from "@/components/ui/button";

export function RouteRecovery({ missing = false }: { missing?: boolean }) {
  const heading = useRef<HTMLHeadingElement>(null);
  const title = missing ? "Page not found" : "This page could not load";
  useEffect(() => {
    document.title = `${title} · Workbench`;
    heading.current?.focus();
  }, [title]);
  return (
    <main className="standalone" aria-labelledby="recovery-title">
      <div className="flex items-center gap-3 text-sm text-muted-foreground">
        {missing ? (
          <Search aria-hidden="true" />
        ) : (
          <AlertCircle aria-hidden="true" />
        )}
        <span>Workbench</span>
      </div>
      <h1 id="recovery-title" ref={heading} tabIndex={-1}>
        {title}
      </h1>
      <p className="leading-relaxed text-muted-foreground">
        {missing
          ? "This address does not match a Workbench page. Open Projects to choose where to continue."
          : "Reload this page to try again. If it still cannot load, share the support code below with your administrator."}
      </p>
      {!missing && (
        <p className="leading-relaxed text-muted-foreground">
          If you were submitting a change, check its status after reloading
          before trying it again.
        </p>
      )}
      <div className="flex flex-wrap gap-3">
        {!missing && (
          <Button onClick={() => window.location.reload()}>Reload page</Button>
        )}
        <Button asChild variant={missing ? "default" : "outline"}>
          <a href="/app/">Open Projects</a>
        </Button>
      </div>
      {!missing && (
        <details className="text-xs text-muted-foreground">
          <summary>Support details</summary>
          <p className="mt-2 font-mono">Code: UI_PAGE_LOAD_FAILED</p>
        </details>
      )}
    </main>
  );
}
