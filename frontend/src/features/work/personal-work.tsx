import type { ReviewScreenshot } from "@/features/delivery/review-screenshots";
import { useUserSession } from "@/features/auth/session";
import {
  createContext,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";

// Private text lives only for this mounted project/session, including route changes.
function usePersonalState() {
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [reviewDrafts, setReviewDrafts] = useState<
    Record<string, { text: string; key: string }>
  >({});
  const [reviewScreenshots, setReviewScreenshots] = useState<
    Record<string, { shots: ReviewScreenshot[]; key: string }>
  >({});
  const titles = useRef(new Map<string, string>());
  const positions = useRef(new Map<string, number>());
  const [lastThread, setLastThread] = useState("");
  return {
    reviewScreenshots,
    setReviewScreenshots,
    reviewDrafts,
    setReviewDrafts,
    drafts,
    setDrafts,
    titles,
    positions,
    lastThread,
    setLastThread,
  };
}
const PersonalWork = createContext<ReturnType<typeof usePersonalState> | null>(
  null,
);
export function PersonalWorkProvider({ children }: { children: ReactNode }) {
  const session = useUserSession();
  const identity =
    session.status === "authenticated" ? session.user.id : session.status;
  return <PersonalSession key={identity}>{children}</PersonalSession>;
}
function PersonalSession({ children }: { children: ReactNode }) {
  const value = usePersonalState();
  return (
    <PersonalWork.Provider value={value}>{children}</PersonalWork.Provider>
  );
}
export function usePersonalWork() {
  const fallback = usePersonalState();
  return useContext(PersonalWork) || fallback;
}
export function useLayoutPreference<T extends string | number>(
  key: string,
  initial: T,
) {
  const [value, update] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(`workbench.layout.${key}`);
      const saved = raw === null ? initial : JSON.parse(raw);
      return typeof saved === typeof initial ? saved : initial;
    } catch {
      return initial;
    }
  });
  const set = (next: T | ((old: T) => T)) =>
    update((old) => {
      const value = typeof next === "function" ? next(old) : next;
      try {
        localStorage.setItem(`workbench.layout.${key}`, JSON.stringify(value));
      } catch {}
      return value;
    });
  return [value, set] as const;
}
