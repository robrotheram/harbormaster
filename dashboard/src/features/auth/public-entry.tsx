import { useEffect, useState } from "react";
import {
  ArrowLeft,
  Check,
  Factory,
  FolderGit2,
  GitPullRequest,
  Layers3,
  LoaderCircle,
  LogIn,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import "./public-entry.css";

function Brand() {
  return (
    <a className="entry-brand" href="/app/" aria-label="Workbench home">
      <span className="entry-mark">
        <Factory aria-hidden="true" />
      </span>
      Workbench
    </a>
  );
}

export function PublicEntry({
  loading,
  error,
  callbackError,
  signIn,
}: {
  loading: boolean;
  error: string;
  callbackError: string;
  signIn: () => Promise<void>;
}) {
  const [redirecting, setRedirecting] = useState(false);
  const [redirectError, setRedirectError] = useState("");
  const login =
    Boolean(callbackError) ||
    new URLSearchParams(window.location.search).get("view") === "login" ||
    !["/", "/app", "/app/"].includes(window.location.pathname);
  useEffect(() => {
    document.title = loading
      ? "Connecting · Workbench"
      : login
        ? "Sign in · Workbench"
        : "Workbench · A home for your agent work";
  }, [loading, login]);

  async function beginSignIn() {
    setRedirecting(true);
    setRedirectError("");
    try {
      await signIn();
    } catch {
      setRedirecting(false);
      setRedirectError("Could not open sign-in. Please try again.");
    }
  }

  return (
    <div className="public-entry">
      <header className="entry-header">
        <Brand />
        {!loading && !login && (
          <Button asChild variant="outline">
            <a href="/app/?view=login">Sign in</a>
          </Button>
        )}
        {!loading && login && (
          <a className="entry-back" href="/app/">
            <ArrowLeft size={16} aria-hidden="true" /> Back to home
          </a>
        )}
      </header>
      {loading ? (
        <main className="entry-session">
          <div className="entry-login-card">
            <ShieldCheck className="entry-symbol" aria-hidden="true" />
            <h1>Checking your session</h1>
            {error ? (
              <>
                <p role="alert">{error}</p>
                <Button
                  onClick={() =>
                    window.dispatchEvent(new Event("workbench:auth-retry"))
                  }
                >
                  Try again
                </Button>
              </>
            ) : (
              <p role="status" className="entry-loading">
                <LoaderCircle
                  className="loading-spinner"
                  size={18}
                  aria-hidden="true"
                />{" "}
                Connecting to your workbench…
              </p>
            )}
          </div>
        </main>
      ) : login ? (
        <main className="entry-login">
          <section className="entry-login-intro">
            <h2>
              Pick up where
              <br />
              you left off.
            </h2>
            <p>
              Your projects, agents, and work.
              <br />
              Together in one place.
            </p>
            <div className="entry-login-note">
              <Layers3 aria-hidden="true" />
              <span>
                From the first task to the final review,
                <br />
                keep the whole project in view.
              </span>
            </div>
          </section>
          <section className="entry-login-card" aria-labelledby="login-title">
            <span className="entry-symbol">
              <LogIn aria-hidden="true" />
            </span>
            <h1 id="login-title">Sign in to your workbench</h1>
            <p>
              Use your approved account to access your projects and workers.
            </p>
            {(callbackError || redirectError) && (
              <p role="alert" className="entry-error">
                {redirectError || callbackError}
              </p>
            )}
            <Button
              className="entry-login-button"
              disabled={redirecting}
              onClick={() => void beginSignIn()}
            >
              {redirecting ? (
                <>
                  <LoaderCircle className="loading-spinner" /> Opening sign-in…
                </>
              ) : (
                <>
                  <LogIn /> Sign in with AuthKit
                </>
              )}
            </Button>
            <p className="entry-secure">
              <ShieldCheck size={16} aria-hidden="true" /> Secure sign-in with
              WorkOS
            </p>
            <div className="entry-access">
              <h2>Need access?</h2>
              <p>
                Ask your Workbench administrator to approve your email address.
              </p>
            </div>
          </section>
        </main>
      ) : (
        <main>
          <section className="entry-hero" aria-labelledby="landing-title">
            <div className="entry-hero-copy">
              <h1 id="landing-title">
                A home for
                <br />
                your agent work.
              </h1>
              <p>
                Bring your projects, coding agents, and workers together. Plan
                the work, follow progress, and review what’s ready.
              </p>
              <Button asChild className="entry-primary">
                <a href="/app/?view=login">Open your workbench</a>
              </Button>
              <span className="entry-caption">
                Sign in with your approved account to get started.
              </span>
            </div>
            <div
              className="entry-work-preview"
              aria-label="Example project workflow"
            >
              <div className="entry-preview-heading">
                <FolderGit2 size={18} aria-hidden="true" />
                <span>Project workbench</span>
                <span className="entry-example">Example</span>
              </div>
              <div className="entry-preview-body">
                <div className="entry-project">
                  <span className="entry-project-icon">
                    <Layers3 aria-hidden="true" />
                  </span>
                  <div>
                    <strong>Website refresh</strong>
                    <span>A shared space for the next release</span>
                  </div>
                </div>
                <div className="entry-task">
                  <span className="entry-task-icon">
                    <Check aria-hidden="true" />
                  </span>
                  <div>
                    <strong>Plan the changes</strong>
                    <span>Give your agent a clear starting point</span>
                  </div>
                  <span className="entry-status">Done</span>
                </div>
                <div className="entry-task">
                  <span className="entry-task-icon">
                    <Terminal aria-hidden="true" />
                  </span>
                  <div>
                    <strong>Build with an agent</strong>
                    <span>Keep the conversation and work together</span>
                  </div>
                  <span className="entry-status">In progress</span>
                </div>
                <div className="entry-task">
                  <span className="entry-task-icon">
                    <GitPullRequest aria-hidden="true" />
                  </span>
                  <div>
                    <strong>Review the result</strong>
                    <span>See what changed before moving forward</span>
                  </div>
                  <span className="entry-status">Up next</span>
                </div>
              </div>
              <div className="entry-preview-footer">
                <ShieldCheck size={15} aria-hidden="true" /> Your project stays
                behind sign-in.
              </div>
            </div>
          </section>
          <section className="entry-features" aria-label="Inside Workbench">
            <article>
              <FolderGit2 aria-hidden="true" />
              <h2>Start with a project</h2>
              <p>
                Keep repositories, tasks, and conversations in the same
                workspace.
              </p>
            </article>
            <article>
              <Terminal aria-hidden="true" />
              <h2>Give agents room to work</h2>
              <p>Run work on connected workers and follow it as it happens.</p>
            </article>
            <article>
              <GitPullRequest aria-hidden="true" />
              <h2>Keep the review close</h2>
              <p>
                Bring changes and feedback back to the work that started them.
              </p>
            </article>
          </section>
        </main>
      )}
      <footer className="entry-footer">
        <span>Workbench</span>
        <span>Projects. Agents. Progress.</span>
      </footer>
    </div>
  );
}
