import { useEffect, useRef } from "react";
import { useLocation } from "@tanstack/react-router";

// Route content can arrive after the shell because pages are lazy-loaded.
// Keep the document title and navigation focus in sync with the rendered page.
export function PageAccessibility() {
  const pathname = useLocation({ select: (location) => location.pathname });
  const previousPath = useRef(pathname);
  useEffect(() => {
    let focusPending = previousPath.current !== pathname;
    previousPath.current = pathname;
    let frame = 0;
    let currentHeading: HTMLElement | null = null;
    const update = () => {
      const heading = currentHeading?.isConnected
        ? currentHeading
        : document.querySelector<HTMLElement>("main h1");
      if (!heading || !heading.getClientRects().length) return;
      // A successful in-place retry may replace an error heading without
      // changing the URL. Keep its title current and recover lost focus.
      if (
        currentHeading &&
        currentHeading !== heading &&
        document.activeElement === document.body
      )
        focusPending = true;
      currentHeading = heading;
      const title = `${heading.textContent?.trim() || "Software factory"} · Workbench`;
      if (document.title !== title) document.title = title;
      if (!focusPending) return;
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (!heading.isConnected) return;
        // Let a closing navigation sheet or menu release its focus trap first.
        if (document.querySelector('[role="dialog"], [role="menu"]')) return;
        focusPending = false;
        observer.disconnect();
        observer.observe(root, {
          childList: true,
          subtree: true,
          characterData: true,
        });
        // Do not interrupt someone who began typing while the page loaded.
        if (
          document.activeElement?.matches(
            "input, textarea, select, [contenteditable=true]",
          )
        )
          return;
        const target = heading.classList.contains("sr-only")
          ? heading.closest<HTMLElement>("main")!
          : heading;
        if (target !== heading)
          target.setAttribute(
            "aria-label",
            heading.textContent || "Page content",
          );
        target.tabIndex = -1;
        target.focus();
      });
    };
    const observer = new MutationObserver(update);
    const root = document.getElementById("root")!;
    // Navigation dialogs can finish closing in a portal outside the app root.
    observer.observe(focusPending ? document.body : root, {
      childList: true,
      subtree: true,
      characterData: true,
      ...(focusPending
        ? {
            attributes: true,
            attributeFilter: ["style", "hidden", "data-state"],
          }
        : {}),
    });
    update();
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [pathname]);
  return null;
}
