import { useEffect, useRef, useState } from "react";
import {
  ChevronsUpDown,
  KeyRound,
  LoaderCircle,
  LogIn,
  LogOut,
  UserRound,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SidebarTooltip } from "@/components/ui/tooltip";
import { profileDetails, useUserSession } from "./session";

export function UserProfileMenu({
  sidebarTooltip = false,
}: { sidebarTooltip?: boolean } = {}) {
  const session = useUserSession();
  const profile = profileDetails(session);
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const running = useRef(false);
  useEffect(() => setError(""), [session.status]);
  const signedIn = session.status === "authenticated";
  const action = signedIn
    ? session.signOut
    : session.status === "anonymous"
      ? session.signIn
      : undefined;

  async function runAction() {
    if (!action || running.current) return;
    running.current = true;
    setPending(true);
    setError("");
    try {
      await action();
      setOpen(false);
    } catch {
      setError(
        signedIn
          ? "Could not log out. Please try again."
          : "Could not sign in. Please try again.",
      );
    } finally {
      running.current = false;
      setPending(false);
    }
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <SidebarTooltip
        label={`${profile.name} (${profile.detail})`}
        enabled={sidebarTooltip}
      >
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="operator-label user-profile-trigger"
            aria-label={`User profile: ${profile.initials ? `${profile.initials} ` : ""}${profile.name} ${profile.detail}`}
          >
            <span className="operator-avatar" aria-hidden="true">
              {session.status === "loading" ? (
                <LoaderCircle className="size-4 loading-spinner" />
              ) : (
                profile.initials || <UserRound size={16} />
              )}
            </span>{" "}
            <span className="rail-label min-w-0 flex-1 text-left">
              <span className="block truncate font-medium">{profile.name}</span>{" "}
              <small className="truncate">{profile.detail}</small>
            </span>
            <ChevronsUpDown
              className="rail-label size-3.5 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          </button>
        </DropdownMenuTrigger>
      </SidebarTooltip>
      <DropdownMenuContent
        side="top"
        align="start"
        sideOffset={8}
        className="w-64 max-w-[calc(100vw-32px)]"
        aria-label="User profile"
        aria-busy={pending}
      >
        <DropdownMenuLabel className="px-3 py-2">
          <span className="text-xs font-normal text-muted-foreground">
            User profile
          </span>
          <span className="mt-1 block break-words">{profile.name}</span>
          <span className="mt-0.5 block break-words text-xs font-normal text-muted-foreground">
            {profile.detail}
          </span>
        </DropdownMenuLabel>
        {session.status === "local" && (
          <p className="px-3 pb-2 text-xs text-muted-foreground">
            You’re using this workbench locally. Sign-in is not configured.
          </p>
        )}
        {error && (
          <p role="alert" className="px-3 pb-2 text-xs text-destructive">
            {error}
          </p>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <a href="/app/connections?tab=agents">
            <KeyRound />
            Connections
          </a>
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={!action || pending}
          onSelect={(event) => {
            event.preventDefault();
            void runAction();
          }}
        >
          {pending ? (
            <LoaderCircle className="loading-spinner" />
          ) : signedIn ? (
            <LogOut />
          ) : (
            <LogIn />
          )}
          {pending
            ? signedIn
              ? "Logging out…"
              : "Signing in…"
            : signedIn
              ? "Log out"
              : "Sign in"}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
