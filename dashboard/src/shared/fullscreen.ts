import { useSyncExternalStore } from "react";

function subscribe(listener: () => void) {
  document.addEventListener("fullscreenchange", listener);
  return () => document.removeEventListener("fullscreenchange", listener);
}

export function useFullscreenElement() {
  return useSyncExternalStore(
    subscribe,
    () => document.fullscreenElement,
    () => null,
  );
}

export function useFullscreenPortal() {
  const element = useFullscreenElement();
  // Radix overlays must live inside the fullscreen surface to remain visible.
  return element instanceof HTMLElement &&
    element.hasAttribute("data-fullscreen-pane")
    ? element
    : undefined;
}

export function exitFullscreenWithin(element: Element | null) {
  if (
    document.fullscreenElement &&
    element?.contains(document.fullscreenElement)
  )
    return document.exitFullscreen().catch(() => {});
}
