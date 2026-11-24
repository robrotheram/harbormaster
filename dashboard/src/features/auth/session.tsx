import { createContext, useContext, type ReactNode } from "react";

export type UserProfile = {
  id: string;
  name: string;
  email?: string;
};

/** Controlled by the authentication integration; local mode is not a session. */
export type UserSession =
  | { status: "local" }
  | { status: "loading" }
  | { status: "anonymous"; signIn: () => Promise<void> }
  | {
      status: "authenticated";
      user: UserProfile;
      signOut: () => Promise<void>;
    };

const SessionContext = createContext<UserSession>({ status: "local" });

export function SessionProvider({
  session = { status: "local" },
  children,
}: {
  session?: UserSession;
  children: ReactNode;
}) {
  return (
    <SessionContext.Provider value={session}>
      {children}
    </SessionContext.Provider>
  );
}

export function useUserSession() {
  return useContext(SessionContext);
}

export function profileDetails(session: UserSession) {
  if (session.status === "authenticated") {
    const name =
      session.user.name.trim() || session.user.email?.trim() || "Your account";
    const words = name.split(/\s+/);
    const initials = [words[0], ...(words.length > 1 ? [words.at(-1)!] : [])]
      .map((word) => Array.from(word)[0])
      .join("")
      .toLocaleUpperCase();
    return { name, initials, detail: session.user.email || "Signed in" };
  }
  if (session.status === "anonymous") {
    return { name: "Guest", initials: null, detail: "Not signed in" };
  }
  if (session.status === "loading") {
    return {
      name: "Your account",
      initials: null,
      detail: "Checking session…",
    };
  }
  return { name: "Local operator", initials: "LO", detail: "Local access" };
}
