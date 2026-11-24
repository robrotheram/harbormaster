import { expect, it } from "vitest";
import { APIError } from "./http";
import {
  accessBlocked,
  pollUnlessBlocked,
  retryTransient,
  refocusUnlessBlocked,
} from "./query-policy";
it("does not repeatedly poll access and deployment restrictions", () => {
  for (const error of [
    new APIError("FORBIDDEN", 403),
    new APIError("SESSION_REQUIRED", 401),
    new APIError("NOT_FOUND_OR_FORBIDDEN", 404),
    new APIError("PROJECT_ISOLATED_WORKERS_REQUIRED", 409),
  ]) {
    expect(accessBlocked(error)).toBe(true);
    expect(retryTransient(0, error)).toBe(false);
    expect(pollUnlessBlocked(5000)({ state: { error } })).toBe(false);
    expect(refocusUnlessBlocked({ state: { error } })).toBe(false);
  }
});
it("keeps transient failures recoverable and resumes polling after recovery", () => {
  const error = new APIError("STORAGE_UNAVAILABLE", 503);
  expect(retryTransient(0, error)).toBe(true);
  expect(retryTransient(1, error)).toBe(false);
  expect(pollUnlessBlocked(5000)({ state: { error } })).toBe(5000);
  expect(pollUnlessBlocked(5000)({ state: { error: null } })).toBe(5000);
  expect(accessBlocked(new APIError("WORK_STATE_CONFLICT", 409))).toBe(false);
});
