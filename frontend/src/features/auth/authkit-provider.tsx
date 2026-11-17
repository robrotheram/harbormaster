import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { PublicEntry } from "./public-entry";
import { SessionProvider, type UserProfile, type UserSession } from "./session";

type ServerSession =
  | { status: "local" | "anonymous" }
  | { status: "authenticated"; user: UserProfile };

function parseSession(value: unknown): ServerSession {
  const data = value as Partial<ServerSession> | null;
  if (data?.status === "local" || data?.status === "anonymous")
    return { status: data.status };
  if (
    data?.status === "authenticated" &&
    data.user &&
    typeof data.user.id === "string" &&
    data.user.id &&
    typeof data.user.name === "string" &&
    typeof data.user.email === "string"
  )
    return data as ServerSession;
  throw new Error("Invalid session response");
}

const callbackErrors: Record<string, string> = {
  access_denied:
    "This account does not have operator access. Sign in with an approved, verified email address.",
  invalid_state:
    "This sign-in attempt expired or was already used. Please sign in again.",
  sign_in_failed: "Sign-in could not be completed. Please try again.",
};

export function AuthKitProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const [session, setSession] = useState<UserSession>({ status: "loading" });
  const [error, setError] = useState("");
  const [callbackError] = useState(() => {
    const url = new URL(window.location.href);
    const code = url.searchParams.get("auth_error");
    if (!code) return "";
    url.searchParams.delete("auth_error");
    window.history.replaceState(window.history.state, "", url);
    return callbackErrors[code] || callbackErrors.sign_in_failed;
  });
  const identity = useRef("");
  const signingOut = useRef(false);
  const generation = useRef(0);
  const signIn = useCallback(async () => {
    window.location.assign("/auth/login");
  }, []);
  const clearPrivateState = useCallback(async () => {
    await client.cancelQueries();
    client.clear();
    // Keep appearance preferences, but discard account-specific navigation.
    try {
      localStorage.removeItem("workbench.last-project");
    } catch {}
  }, [client]);
  const signOut = useCallback(async () => {
    if (signingOut.current) return;
    signingOut.current = true;
    generation.current++;
    try {
      const response = await fetch("/auth/logout", {
        method: "POST",
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
        headers: { "X-Workbench-Request": "1" },
        signal: AbortSignal.timeout(20000),
      });
      if (!response.ok) throw new Error("Logout failed");
      const data = await response.json();
      const target = new URL(data.url, window.location.origin);
      if (
        target.origin !== window.location.origin &&
        target.origin !== "https://api.workos.com"
      )
        throw new Error("Invalid logout URL");
      setSession({ status: "anonymous", signIn });
      identity.current = "anonymous";
      await clearPrivateState();
      window.location.assign(target.href);
    } finally {
      signingOut.current = false;
    }
  }, [clearPrivateState, signIn]);

  useEffect(() => {
    const controller = new AbortController();
    let pending = false;
    async function refresh() {
      if (pending || signingOut.current || controller.signal.aborted) return;
      pending = true;
      const requestGeneration = generation.current;
      try {
        const response = await fetch("/auth/session", {
          credentials: "same-origin",
          cache: "no-store",
          redirect: "error",
          signal: AbortSignal.any([
            controller.signal,
            AbortSignal.timeout(20000),
          ]),
        });
        if (!response.ok) throw new Error("Session unavailable");
        const data = parseSession(await response.json());
        if (
          controller.signal.aborted ||
          signingOut.current ||
          generation.current !== requestGeneration
        )
          return;
        const nextIdentity =
          data.status === "authenticated" ? data.user.id : data.status;
        if (identity.current && nextIdentity !== identity.current) {
          setSession({ status: "loading" });
          await clearPrivateState();
        }
        if (
          controller.signal.aborted ||
          signingOut.current ||
          generation.current !== requestGeneration
        )
          return;
        identity.current = nextIdentity;
        setError("");
        setSession(
          data.status === "authenticated"
            ? { ...data, signOut }
            : data.status === "anonymous"
              ? { status: "anonymous", signIn }
              : { status: "local" },
        );
      } catch {
        if (
          !controller.signal.aborted &&
          !signingOut.current &&
          generation.current === requestGeneration
        ) {
          setError(
            "Could not check your session. Check your connection and try again.",
          );
          setSession({ status: "loading" });
        }
      } finally {
        pending = false;
      }
    }
    function expired() {
      generation.current++;
      identity.current = "anonymous";
      setError("");
      setSession({ status: "anonymous", signIn });
      void clearPrivateState();
    }
    function visible() {
      if (document.visibilityState === "visible") void refresh();
    }
    void refresh();
    const timer = window.setInterval(visible, 60000);
    window.addEventListener("focus", visible);
    window.addEventListener("workbench:auth-required", expired);
    window.addEventListener("workbench:auth-retry", visible);
    return () => {
      controller.abort();
      window.clearInterval(timer);
      window.removeEventListener("focus", visible);
      window.removeEventListener("workbench:auth-required", expired);
      window.removeEventListener("workbench:auth-retry", visible);
    };
  }, [clearPrivateState, signIn, signOut]);

  if (session.status === "loading" || session.status === "anonymous") {
    return (
      <PublicEntry
        loading={session.status === "loading"}
        error={error}
        callbackError={callbackError}
        signIn={signIn}
      />
    );
  }

  return <SessionProvider session={session}>{children}</SessionProvider>;
}
