// External bootstrap keeps the saved theme compatible with script-src 'self'.
try {
  document.documentElement.classList.toggle(
    "dark",
    localStorage.getItem("workbench.theme") !== "light",
  );
} catch {}
