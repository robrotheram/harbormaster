import html2canvas from "html2canvas";

type Region = { x: number; y: number; width: number; height: number };
type Drag = { id: number; x: number; y: number; box: HTMLDivElement };
type Anchor = {
  element: Element;
  x: number;
  y: number;
  width: number;
  height: number;
  path: string;
  box: HTMLDivElement;
};

function install(origin: string) {
  const container = document.createElement("div");
  container.setAttribute("data-html2canvas-ignore", "");
  container.style.cssText =
    "all:initial;position:fixed;inset:0;z-index:2147483647;pointer-events:none;overflow:hidden";
  const shadow = container.attachShadow({ mode: "open" });
  document.documentElement.append(container);
  const anchors = new Map<string, Anchor>();
  let drag: Drag | undefined;
  let nonce = "";
  let active = false;
  let placing = false;
  let frame = 0;

  function box() {
    const element = document.createElement("div");
    element.style.cssText =
      "position:absolute;box-sizing:border-box;border:2px solid #254ade;box-shadow:0 0 0 1px white;background:#4775ff22";
    shadow.append(element);
    return element;
  }
  function position(element: HTMLDivElement, region: Region) {
    Object.assign(element.style, {
      left: `${region.x}px`,
      top: `${region.y}px`,
      width: `${region.width}px`,
      height: `${region.height}px`,
    });
  }
  function paint() {
    frame = 0;
    for (const anchor of anchors.values()) {
      anchor.box.hidden =
        anchor.path !== location.pathname || !anchor.element.isConnected;
      if (anchor.box.hidden) continue;
      const rect = anchor.element.getBoundingClientRect();
      position(anchor.box, {
        ...anchor,
        x: rect.x + anchor.x,
        y: rect.y + anchor.y,
      });
    }
  }
  function schedulePaint() {
    if (!frame) frame = requestAnimationFrame(paint);
  }
  function clearDrag() {
    drag?.box.remove();
    drag = undefined;
  }
  function region(start: Drag, end: PointerEvent): Region {
    const x = Math.max(0, Math.min(innerWidth, end.clientX));
    const y = Math.max(0, Math.min(innerHeight, end.clientY));
    return {
      x: Math.min(start.x, x),
      y: Math.min(start.y, y),
      width: Math.abs(start.x - x),
      height: Math.abs(start.y - y),
    };
  }
  function report(type: string, data: object = {}) {
    parent.postMessage({ type, nonce, ...data }, origin);
  }
  window.addEventListener("message", (event) => {
    if (
      event.source !== parent ||
      event.origin !== origin ||
      event.data?.type !== "workbench-review"
    )
      return;
    const data = event.data;
    if (typeof data.nonce !== "string" || !Array.isArray(data.areas)) return;
    nonce = data.nonce;
    active = data.enabled === true;
    if (!active) clearDrag();
    for (const [id, anchor] of anchors)
      anchor.box.hidden = !data.areas.some(
        (area: { id: string }) => area.id === id,
      );
    data.areas.forEach(({ id, number }: { id: string; number: number }) => {
      const anchor = anchors.get(id);
      if (!anchor || !Number.isInteger(number)) return;
      if (!anchor.box.firstChild) {
        const label = document.createElement("span");
        label.style.cssText =
          "display:grid;place-items:center;width:22px;height:22px;color:white;background:#254ade;font:600 13px system-ui";
        anchor.box.append(label);
      }
      anchor.box.dataset.reviewArea = String(number);
      anchor.box.firstElementChild!.textContent = String(number);
    });
    for (const [id, anchor] of anchors) {
      if (anchor.box.hidden) {
        anchor.box.remove();
        anchors.delete(id);
      }
    }
    paint();
  });
  window.addEventListener(
    "scroll",
    () => {
      clearDrag();
      schedulePaint();
    },
    true,
  );
  window.addEventListener("resize", schedulePaint);
  window.addEventListener(
    "pointerdown",
    (event) => {
      if (!active || placing || drag || event.button !== 0 || !event.isPrimary)
        return;
      event.preventDefault();
      event.stopImmediatePropagation();
      drag = {
        id: event.pointerId,
        x: event.clientX,
        y: event.clientY,
        box: box(),
      };
      position(drag.box, region(drag, event));
    },
    true,
  );
  window.addEventListener(
    "pointermove",
    (event) => {
      if (drag?.id !== event.pointerId) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      position(drag.box, region(drag, event));
    },
    true,
  );
  window.addEventListener("pointercancel", clearDrag);
  window.addEventListener("blur", clearDrag);
  window.addEventListener(
    "click",
    (event) => {
      if (!active) return;
      event.preventDefault();
      event.stopImmediatePropagation();
    },
    true,
  );
  window.addEventListener(
    "pointerup",
    async (event) => {
      if (drag?.id !== event.pointerId) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      const bounds = region(drag, event);
      clearDrag();
      if (bounds.width < 8 || bounds.height < 8) return;
      const id = crypto.randomUUID();
      const path = location.pathname;
      const element =
        document.elementFromPoint(
          bounds.x + bounds.width / 2,
          bounds.y + bounds.height / 2,
        ) || document.documentElement;
      const rect = element.getBoundingClientRect();
      const left = Math.floor(Math.max(0, bounds.x - 40));
      const top = Math.floor(Math.max(0, bounds.y - 40));
      const width = Math.ceil(
        Math.min(innerWidth, bounds.x + bounds.width + 40) - left,
      );
      const height = Math.ceil(
        Math.min(innerHeight, bounds.y + bounds.height + 40) - top,
      );
      const request = nonce;
      placing = true;
      report("workbench-review-capturing");
      try {
        const image = await html2canvas(document.documentElement, {
          x: scrollX + left,
          y: scrollY + top,
          width,
          height,
          scrollX,
          scrollY,
          windowWidth: innerWidth,
          windowHeight: innerHeight,
          scale: Math.min(1, 1200 / Math.max(width, height)),
          logging: false,
          imageTimeout: 3000,
          useCORS: true,
        });
        if (!active || nonce !== request || location.pathname !== path) return;
        const url = image.toDataURL("image/jpeg", 0.8);
        if (
          !url.startsWith("data:image/jpeg;base64,") ||
          url.length > 1_398_100
        )
          throw new Error("Image too large");
        anchors.set(id, {
          ...bounds,
          box: box(),
          element,
          path,
          x: bounds.x - rect.x,
          y: bounds.y - rect.y,
        });
        report("workbench-review-area", {
          id,
          path,
          url,
          outline: {
            x: (bounds.x - left) / width,
            y: (bounds.y - top) / height,
            width: bounds.width / width,
            height: bounds.height / height,
          },
        });
        paint();
      } catch {
        if (active && nonce === request) report("workbench-review-failed");
      } finally {
        placing = false;
      }
    },
    true,
  );
}

const parentOrigin = document.currentScript?.getAttribute("data-parent-origin");
if (parentOrigin && parent !== window) install(parentOrigin);
