import { APIError } from "./http";

export function accessBlocked(error: unknown) {
  return (
    error instanceof APIError &&
    ([401, 403, 404].includes(error.status) ||
      error.code === "PROJECT_ISOLATED_WORKERS_REQUIRED")
  );
}
export const retryTransient = (failures: number, error: unknown) =>
  !accessBlocked(error) && failures < 1;
export const pollUnlessBlocked =
  (interval: number) => (query: { state: { error: unknown } }) =>
    accessBlocked(query.state.error) ? (false as const) : interval;
export const refocusUnlessBlocked = (query: { state: { error: unknown } }) =>
  !accessBlocked(query.state.error);
