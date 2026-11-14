export function workspacePath(value: string) {
  try {
    const path = decodeURIComponent(value).replace(/^\.\//, "");
    if (
      !path ||
      path.length > 512 ||
      /[\\\x00-\x1f:?#]/.test(path) ||
      path.startsWith("/")
    )
      return undefined;
    if (
      path
        .split("/")
        .some(
          (p) =>
            !p ||
            [".", "..", ".git", ".t3", ".codex", ".claude"].includes(p) ||
            p === ".env" ||
            p.startsWith(".env."),
        )
    )
      return undefined;
    return path;
  } catch {
    return undefined;
  }
}
export const imagePath = (value: string) => {
  const path = workspacePath(value);
  return path && /\.(svg|png|jpe?g|gif|webp|avif)$/i.test(path)
    ? path
    : undefined;
};
export const dataPath = (value: string) => {
  const path = workspacePath(value);
  return path && /\.(csv|json)$/i.test(path) ? path : undefined;
};

// Only explicitly marked chart files are embedded in conversation messages.
export const chartPath = (value: string) => {
  const path = workspacePath(value);
  return path && /\.chart\.json$/i.test(path) ? path : undefined;
};
