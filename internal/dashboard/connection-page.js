"use strict";
(() => {
  const page = document.getElementById("connection");
  if (!page) return;
  const heading = document.getElementById("heading");
  const messages = JSON.parse(page.dataset.messages);
  const initialTitle = heading.textContent;
  const initialBody = document.getElementById("status").textContent;
  function fail(code, status) {
    if (typeof code !== "string" || !Object.hasOwn(messages, code))
      code = "CONNECTION_UNAVAILABLE";
    const message = messages[code];
    heading.textContent = message.title;
    document.title = message.title + " · Workbench";
    document.getElementById("status").textContent = message.body;
    document.getElementById("support-code").textContent = code;
    document.getElementById("http-label").hidden = !status;
    document.getElementById("http-status").hidden = !status;
    document.getElementById("http-status").textContent = status || "";
    document.getElementById("support").hidden = false;
    const recovery = document.getElementById("embedded-recovery");
    if (recovery) recovery.hidden = false;
    if (page.dataset.embedded !== "true") heading.focus();
  }
  if (page.dataset.code) {
    if (page.dataset.embedded !== "true") heading.focus();
    return;
  }
  let active;
  async function connect() {
    const ticket = new URLSearchParams(location.hash.slice(1)).get("ticket");
    history.replaceState(null, "", "/connect");
    active?.abort();
    const controller = new AbortController();
    active = controller;
    if (!ticket) {
      fail("OPEN_FROM_WORKBENCH");
      return;
    }
    heading.textContent = initialTitle;
    document.title = initialTitle + " · Workbench";
    document.getElementById("status").textContent = initialBody;
    document.getElementById("support").hidden = true;
    const recovery = document.getElementById("embedded-recovery");
    if (recovery) recovery.hidden = true;
    const timeout = setTimeout(() => controller.abort(), 20000);
    try {
      const response = await fetch("/connect", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Workbench-Request": "1",
        },
        body: JSON.stringify({ ticket }),
        signal: controller.signal,
      });
      if (active !== controller) return;
      if (response.ok) {
        location.replace("/");
        return;
      }
      const data = await response.json().catch(() => ({}));
      if (active !== controller) return;
      if (controller.signal.aborted) throw new Error("Connection timed out");
      fail(data?.code, response.status);
    } catch {
      if (active === controller)
        fail(
          controller.signal.aborted ? "CONNECTION_TIMEOUT" : "NETWORK_ERROR",
        );
    } finally {
      clearTimeout(timeout);
    }
  }
  window.addEventListener("hashchange", connect);
  void connect();
})();
