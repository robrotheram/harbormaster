import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve, dirname } from "node:path";
import { describe, it, expect } from "vitest";
const root = fileURLToPath(new URL("../", import.meta.url));
const sourceFiles = readdirSync(root, { recursive: true })
  .filter(
    (name) =>
      /\.(ts|tsx)$/.test(String(name)) && !/\.test\./.test(String(name)),
  )
  .map(String);
function imports(file: string) {
  return [
    ...readFileSync(resolve(root, file), "utf8").matchAll(
      /(?:from\s*|import\s*\()\s*["']([^"']+)["']/g,
    ),
  ].map((match) => match[1]);
}
describe("frontend dependency boundaries", () => {
  it("keeps reusable infrastructure independent of application features", () => {
    const violations = sourceFiles
      .filter((file) => /^(shared|components|lib)\//.test(file))
      .flatMap((file) =>
        imports(file)
          .filter(
            (name) =>
              name.startsWith("@/features/") || name.startsWith("@/app/"),
          )
          .map((name) => `${file} -> ${name}`),
      );
    expect(violations).toEqual([]);
  });
  it("keeps feature API and model modules independent of screen implementations", () => {
    const violations = sourceFiles
      .filter(
        (file) =>
          /^features\//.test(file) &&
          /(?:^|\/)(?:[^/]*api|model|policy)\.ts$/.test(file),
      )
      .flatMap((file) =>
        imports(file)
          .filter((name) => {
            const target = name.startsWith("@/")
              ? resolve(root, name.slice(2))
              : name.startsWith(".")
                ? resolve(root, dirname(file), name)
                : undefined;
            return (
              target &&
              sourceFiles.some(
                (candidate) => resolve(root, candidate) === target + ".tsx",
              )
            );
          })
          .map((name) => `${file} -> ${name}`),
      );
    expect(violations).toEqual([]);
  });
});
