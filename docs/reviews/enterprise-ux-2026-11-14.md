# Enterprise UX review — 14 November 2026

> Historical record. See the [documentation index](../README.md) for current setup and workflows.

Status: thirteenth polish iteration complete. The user accepted 9/10 as the release threshold and requested deployment; the dashboard is deployed and post-deployment verification passed on 14 November 2026. This is an evidence-based design assessment, not production certification or a prediction of investor interest.

## Method

Reviewed all 14 active main routes through the T3 browser against the running application on port 8080, then the edited frontend on port 5173 with the same live read APIs. The compiled production bundle was also reviewed on port 5174 using the production CSP. The initial review left port 8080 unchanged; the accepted release was subsequently deployed there as recorded below. Desktop review used 1280×800 and 1440×900; phone review used 390×844. Automated consistency coverage additionally uses 320px and 1440px. Examined populated, empty, expired-preview, missing-page, worker-readiness, account-setup and connection-profile states. Simulated a network failure in the review tab only, then restored networking. No live approval, deletion, provider-account change or work submission was performed.

Scores weigh task clarity, hierarchy, consistency, accessibility and failure recovery equally. A 10 requires no material known defects in the reviewed flows and verified recovery, keyboard and responsive behavior. Scores below reflect thirteen iterations, including two secondary connection screens found outside the main navigation. Integration, accessibility and operational acceptance gates remain open; the initially suspected routing defect was traced to the browser test harness, as detailed below.

## Page scores

| Page | Initial /10 | Current /10 | Findings and remaining work |
| --- | ---: | ---: | --- |
| Projects | 7.5 | 9.5 | Paged inventory limits live status queries to displayed rows; full-portfolio search shows matching counts. A 120-project fixture passed search, pagination and keyboard checks. Creation rejects blank names, locks pending controls and preserves failed drafts. Real server load and first-time-user acceptance remain open. |
| Issue board | 7.5 | 9 | Added direct mobile stage navigation with counts, keyboard support and reduced-motion handling. Failed loads offer retry instead of empty lanes. Verified at 320/390px; first-time-user task observation remains open. |
| Reviews | 8 | 9.5 | Failed/loading queues and evidence no longer imply empty results. Read retries preserve context. Comments survive task switches within the mounted project/attempt scope, enforce the 16 kB UTF-8 limit and reuse delivery keys on unchanged retries. Failed status reads disable decisions while preserving drafts. Populated phone/theme and enlarged-text checks pass. A 120-task fixture verifies bounded title loading, full-queue search, title retry and stable selection/drafts while filtering. Live review and assistive-technology acceptance remain open. |
| Work | 7 | 9.5 | New-thread and draft forms explain byte/output limits, preserve drafts, and expose field errors accessibly. Read retries cannot submit the form. Project-list refresh failures now preserve the current conversation and unsaved draft, with a compact warning and retry. Large lists now page 20 threads at a time and read titles in batches of four; search covers all matching-status titles with progress and incomplete-result recovery. A 200-thread fixture and a 63-event conversation verify search, drafts, keyboard scrolling and reading-position preservation. Extended-duration and assistive-technology acceptance remain open. |
| Workers | 8 | 9.5 | Worker terminology is consistent across inventory/actions/forms. Pending creation/settings/copy/start controls are locked; profile lookup failures retry without saving. Deletion uses exact-name confirmation and retains failed context. Four dialogs passed narrow-screen accessibility checks in both themes. History exports now preserve the app on failure, reject incomplete export envelopes and cancel when closed. Log failures preserve context and distinguish cached observations. Live operational acceptance remains separate. |
| Outputs & previews | 7 | 9 | Explained recipes and saved previews, bounded Unicode-safe output reader, cleared stale content on worker changes, guarded lifecycle actions when status is unavailable, closed failed blank preview tabs. Job, file and preview status failures retry in their own section without starting work. Synthetic lifecycle and recovery tests pass; live recipe acceptance remains open. |
| Agent homes | 8 | 9 | Consistent worker terminology, singular counts and in-place retry; distinguishes failed from empty inventory. Worker-name lookup failures now explain partial data and retry without hiding home records. Validate understanding of persistence with inexperienced users. |
| Hermes setup | 7.5 | 9 | Credential checks lock related edits; failed observations show unknown status and guard actions. Incomplete settings explain how to recover across tabs; profile retry preserves drafts. Both tabs passed responsive, keyboard and automated accessibility checks. Live Matrix acceptance remains open. |
| Project settings | 8 | 9 | Standard application dialog with exact project-name confirmation, cancellation/focus return and inline failure recovery. Verified retained-disk wording against backend deletion implementation. No live deletion performed. |
| Infrastructure | 8 | 9 | Corrected CPU plural; allocations distinguished from utilization. Partial observations and bulk failures have browser coverage, including the final full regression. Deployment/load acceptance remains separate. |
| Platform services | 7.5 | 9 | Failed/missing health checks explain the next action, diagnostics disclose support codes, and per-service observation times show freshness. Outage recovery and narrow layout verified with controlled responses. |
| Connection profiles | 8 | 9.5 | Failed loads no longer claim no search matches. Added in-place retry and meaningful first-profile guidance. Responsive form and deletion tests pass. Actual policy enforcement remains a separate acceptance gate. |
| Repository connections | 7 | 9 | Account and connection errors retry in place. Disconnection confirms the named target, retains context after failure and explains imported-work impact. Repository access checks lock submitted fields and preserve failed drafts; ticket forms guard pending submission. Real provider authorization remains a separate integration gate. |
| Getting started | 6.5 | 9 | Improved reading size, added direct Projects action, aligned worker terminology and identified host-side credential repair as an administrator task. First-time-user task observation remains open. |

## Secondary connection screens

| Screen | Initial /10 | Current /10 | Findings and remaining work |
| --- | ---: | ---: | --- |
| Preview connection and expired session | 3 | 9 | Replaced an unstyled paragraph and raw navigation errors with a responsive recovery card, meaningful heading/title, support details and a Workbench action. Network failures and bounded timeouts differ from expired links. Embedded recovery preserves editor focus and points to parent controls. Live integration acceptance remains open. |
| Worker browser connection | 3 | 9 | Uses the same recovery layout and worker terminology. Missing links make no exchange request; fresh links pasted into an expired tab work. Real handler tests preserve one-use ticket exchange, origin restrictions, cookies, redirects and API JSON errors. Native T3 availability and first-time-user acceptance remain separate. |

## Shared fixes

- Network, timeout, expired-session, throttling and unavailable-service errors now explain an appropriate next step. Uncertain outcomes advise checking state before repeating a change.
- API errors expose code/status in expandable support details without exposing raw server bodies.
- Work conflicts, unavailable instructions, missing output and uncertain instruction publication have specific guidance.
- Missing project and retired-page states offer direct recovery links with page headings. Unknown routes have a main landmark and a phone gutter.
- Saved theme initialization moved from blocked inline JavaScript to a same-origin script compatible with the production content security policy. Small fonts are emitted as same-origin files instead of CSP-blocked data URLs.
- Existing smoke tests were reconciled with intentional scheduling-page removal and the addition of Repositories to the Platform menu.

## Second-iteration fixes

- Outputs now explain the start-worker side effect, separate saved preview workers from source workers, show friendly recipe names and readable file sizes, and put SHA-256 under verification details. The reader decodes UTF-8 correctly, wraps long content and rejects binary data explicitly.
- Preview open/revoke/release controls require a successful current status observation. Failed connections close their temporary blank tab. Worker switches clear prior output.
- Board stages are directly reachable on phones through readable buttons with counts. The search icon is vertically centered; unavailable counts are not reported as zero. Initial load failures cannot masquerade as empty lanes, and retry is available in place.
- Reviews and profiles distinguish unavailable data from empty results. Shared error notices support explicit read retries, also used by Agent homes, Hermes setup and Outputs. Error copy uses the same worker terminology as navigation.
- Project deletion uses the standard focus-managed dialog with exact-name confirmation and retained-data details. Failures preserve context; cancellation restores focus. Backend `internal/api/deletion.go` and `internal/store/deletion.go` confirm that private disks remain on the host.
- T3 inspection confirmed the revised output layout, mobile board navigation and project-deletion dialog against the compiled bundle. Live data remained unchanged.

## Third-iteration fixes

- Added axe-core WCAG A/AA checks to the full-page consistency test. The test uses long project, worker and profile names at 1440px and 320px, in dark and light themes.
- Corrected mismatches between visible and accessible navigation names; decorative project initials became a folder icon. Empty board scrolling is keyboard accessible.
- Long project names no longer push the Workers page beyond the phone viewport. Light-theme operator initials and destructive controls received stronger contrast after actual failures were detected.
- New work and advanced drafts explain oversized instructions and excess output paths. Invalid fields expose their error descriptions; pending draft creation locks its inputs. Blank instructions cannot submit.
- Read retries use explicit button types, preventing them from accidentally submitting a surrounding form. Added a regression check for draft preservation and zero work submissions during worker-list recovery.
- Service diagnostics explain failed and missing probes, retain technical codes under support details, and show per-service check times. Repository disconnection uses an in-app confirmation and preserves failure context.
- Getting started uses more readable body text, links directly to Projects, and distinguishes host-side administrator repair from worker activity. No separate walkthrough was created.

## Fourth-iteration fixes

- Hermes credential checks lock related settings, token and invitation inputs until the request completes, avoiding lost edits and conflicting operations.
- Incomplete Matrix settings and unavailable saved profiles explain which tab needs attention before saving. Profile lookup failures distinguish an unverified selection from a confirmed missing profile and offer in-place retry.
- Failed runtime observations preserve drafts, disable state-changing actions and replace stale running/connected claims with unknown status. Recovery restores controls without submitting edits.
- Dark/light checks now include both Hermes tabs with unsaved settings, covering credential controls and warning contrast beyond the original whole-page Agent-tab scan.

## Fifth-iteration fixes

- Projects now show 20 rows per page. Only displayed rows subscribe to live work/worker queries; search still covers the full portfolio. Matching counts, singular wording and whitespace-tolerant search improve clarity.
- Project creation rejects whitespace-only names and guards keyboard submission. Pending creation locks the name, cancel and close controls; failures retain the draft. Guidance uses the shared worker terminology.
- Shared dialog guidance uses proportional line height. Actions wrap and grow with enlarged text, form controls retain enough height, and the close target is 24px. Text-only enlargement at 320px remains scrollable without horizontal overflow.
- Added checks for mobile navigation focus containment, Escape/focus return, large-project menu keyboard access, and all-page text spacing at desktop and phone widths.

## Sixth-iteration fixes

- Standardized worker wording across creation, branching, settings, file copying and inventory actions. Worker creation rejects whitespace-only names; configuration and file-copy forms lock controls while pending and preserve failed drafts.
- Profile failures retain the selected identity, distinguish unknown from missing, and support read retry without submitting settings. Changing a file-copy source clears an identical destination instead of leaving hidden invalid state.
- Replaced native worker-deletion confirmation with the shared dialog, exact-name confirmation, pending close lock and inline error recovery. Wording reflects backend behavior: deletion first stops the worker, refuses active/unknown runtime state, and retains private disks. Source and deletion regression tests were inspected in `internal/api/deletion.go` and `internal/api/deletion_test.go`; no live deletion was performed.
- Pending project deletion, repository disconnection and advanced work creation now visibly disable the dialog close control. Repository access checks and ticket creation lock their submitted fields; empty ticket pages no longer offer endless Next navigation.
- Worker dialog scanning found real light-theme contrast failures on muted surfaces. Darkened the shared light-theme secondary text token to #666666; dark-theme colors were preserved.

## Seventh-iteration fixes

- Message history now handles export failures inside its dialog instead of navigating the application to an API error. Downloads require the expected format, project/worker manifest and complete export footer; interrupted exports are rejected. Closing a pending export aborts the read. This checks the export envelope, not its cryptographic digest.
- History startup locks worker selection until recovery completes and uses a fresh request key after successful starts. Newest resets the cursor without also refetching the old page. Message metadata wraps on phones and the scroll region is keyboard accessible.
- Activity logs no longer claim no activity after a failed request. Errors retry in place; cached events retain a last-successful-update indicator, and stale runtime summaries are hidden. Pause disables interval, focus and reconnect refreshes.
- T3 phone inspection found collapsed worker selectors and cramped log columns despite passing automated accessibility checks. Selectors now get a full row at phone widths; activity events use compact two-row layouts on phones and retain table columns on wider screens. Displayed event sources use the same human-readable names as the filter. The browser check asserts a readable selector width.
- The final dialog check caught insufficient light-theme contrast on hovered primary buttons. Shared primary buttons now darken with an opaque color mix instead of becoming translucent; the accessibility check explicitly hovers the export action in both themes.
- Saved jobs/files/preview observations now retry in the relevant section. Agent homes explain unavailable worker details while preserving home records. Failed pairing-link clipboard access offers manual-copy guidance.

## Eighth-iteration fixes

- All 14 main pages now identify themselves in the browser title. Client-side navigation moves focus to the new heading after navigation overlays close; an already-focused editor keeps focus while the page finishes loading. Browser back navigation is covered. Skip to content targets a programmatically focusable main region.
- Page title updates do not wait for animation frames, allowing background tabs to identify loaded content. A view with a visually hidden heading uses its main region as the focus target. The initial implementation disconnected its temporary content observer after the route settled. The ninth iteration adds a lightweight heading observer for in-place recovery, while dropping style-attribute observation once focus settles.
- Header navigation/theme icon targets keep stable touch sizes instead of doubling with the base font and crowding the breadcrumb. The Docs action can grow vertically with its text. Phones show the project name in the header and the page name in the content, avoiding a squeezed duplicate breadcrumb.
- Removed the font-dependent body minimum width that expanded to 640px with a doubled base font, forcing every 320px page to scroll horizontally.
- Getting started now stacks guide icons above text on phones and allows content columns to shrink without clipping. Hermes title/badge, tabs and long setup actions reflow with enlarged text; action wrapping is applied after utility layers so it actually overrides fixed one-line styles. Shorter “Save settings” and “Start Hermes” actions reduce wrapping while retaining context.

## Ninth-iteration fixes

- Failed page modules and missing routes use a consistent recovery card, page-specific browser title, focused heading and standard action buttons. Reload actually reloads the document, clearing a failed lazy-module import; Open Projects offers another route. Failure guidance covers uncertain submitted changes, with a stable UI support code behind a disclosure.
- An initial project-list outage now has a page heading, an in-place retry and a Help action. Independent pages remain usable and identify the unavailable project navigation. The sidebar offers Retry project list rather than implying the user has no projects and needs to create one.
- Failed refreshes preserve cached project views and open drafts. A compact, keyboard-scrollable warning keeps the retry action visible on phones; detailed causes and technical codes remain in Support details. Query retries are reads and do not submit work.
- Page titles and lost focus recover when an error view is replaced at the same URL. The heading observer reuses the existing heading reference and stops watching style attributes after navigation focus settles.
- Storage failures now ask inexperienced users to share support details with an administrator rather than instructing them to change database migrations. Service guidance uses the actual Platform → Platform services label.

## Tenth-iteration fixes

- Work initially reads titles for the displayed 20 threads and any selected off-page thread. Show more extends the list by 20. Search trims surrounding whitespace and scans every title within the chosen status in batches of four, with progress and honest incomplete-result guidance. Failed titles have a read-only retry; no worker is started automatically.
- Private title caches include the saved specification digest. Unchanged titles survive ordinary list refreshes without a new file read; a changed follow-up digest refreshes its title. Caches are discarded on unmount, and retries stop admitting batches when the scope changes or the view closes.
- Loading older messages no longer incorrectly announces New activity. Notices follow newly recorded event sequences. The keyboard-focusable message region preserves the reader's position during new updates; the explicit return-to-latest action respects reduced motion.
- The phone thread drawer gives retry and pagination controls the same 44px minimum height as its other actions. Counts distinguish available matches after a title failure. Thread button names include their visible title, state and date rather than overriding that content with a shorter ARIA label.

## Eleventh-iteration fixes

- Secondary preview/worker connection pages now share a readable, responsive layout with light/dark system palettes, a page heading, contextual recovery, keyboard focus, 44px actions and expandable support details. These embedded Go assets work without the main frontend build. Navigation errors render HTML while API/resource and ticket-exchange errors retain their HTTP/JSON contract.
- Missing links do not attempt a POST. Expired/used links, interrupted networks, unavailable services and a 20-second timeout have distinct recovery guidance. Raw server bodies and unknown codes are not shown. A new link pasted into the same expired tab starts a fresh exchange; an older interrupted request cannot overwrite the new state.
- Embedded preview recovery keeps the iframe sandbox unchanged. It uses the parent Reload preview / Preview options controls rather than offering a blocked popup, and it does not steal focus from an unsent message. Standalone recovery opens the fixed local Workbench origin in a separate tab with no opener/referrer.

## Twelfth-iteration fixes

- Review comments and their delivery keys survive task switches within the mounted project view, scoped to the work item and reviewed attempt. Draft markers identify unsent changes; edited comments get new keys. Leaving the project discards these private drafts, and a new attempt does not silently inherit feedback intended for the previous result.
- Review comments expose the same 16 kB UTF-8 limit as conversation follow-ups, with an accessible inline error and guarded submission. Pending controls stay locked. A failed queue or attempt status read disables approval, closure and submission while keeping the current comment editable.
- Request, attempt, recorded-evidence, task-result and retained-file reads have in-place retry. Loading/failed evidence has a distinct state instead of falsely claiming that no report, edits or visualisations exist. Cached evidence is identified when its refresh fails; unrelated failed result fallback is no longer presented after recorded messages recover.
- Phone review actions, mode tabs, queue controls and error retries have 44px minimum targets. The comment's visible label now matches its accessible name. History failures no longer speculate about restarting into another image; history and final-response messages point to relevant status checks and support details.

## Thirteenth-iteration fixes

- Reviews now renders 20 tasks initially, with explicit counts and Show more tasks. Private title reads use the shared four-at-a-time, digest-keyed loader; the open review has its own independent request-text read. Search covers the entire selected queue and explains loading, failed title reads and incomplete matches, with in-place retry.
- Filtering no longer silently changes the open review or its comment. The selected review is retained until an explicit task/mode change or removal from that queue. A Show open review action clears the filter, expands the list as needed, scrolls to the selected task and restores keyboard focus to it. On phones, matches precede this notice so recovery guidance does not obscure the first result.
- Title retries and list recovery use readable foreground text and 44px controls. Private comments remain scoped to the mounted project and reviewed attempt; no live decision or lifecycle action was exercised.

## Validation log

- Thirteenth iteration: the existing 12 Reviews browser scenarios passed after the queue changes. The new 120-task test verifies 20/40-row paging, full title coverage, at most four concurrent title reads during search (plus the independent active-review read on initial load), partial-result guidance/retry, whitespace-tolerant search, preserved selection/comments, explicit switching, and returning to an off-page review. Four populated desktop/phone/theme axe checks and overflow checks pass. The final focused test also verifies scrolling and keyboard focus after Show open review. Production build/TypeScript, full formatting and final changed-file formatting/whitespace checks passed.
- The new test initially caught a contrast failure during a theme transition. Queue action text now uses the foreground color, and the audit switches themes through the actual UI and waits for CSS transitions as the existing tests do; no rule was suppressed. T3 independently confirmed a 120-task read-only queue, failed-title recovery, the unchanged open review/comment, and no horizontal page overflow at 320px. A fresh compiled phone inspection prompted moving the outside-list notice below matching tasks. No live data or decisions changed.

- Twelfth iteration: all 12 Reviews browser scenarios passed, including four new tests for byte limits/draft switching/stable retry keys, cached status outages, failed-versus-empty evidence and retained-file retry. After the mobile target-size and shared error-copy changes, all four new tests passed again. The open-comment state passed axe in both themes at 1440px and 320px, plus doubled-base-text overflow/button-clipping checks. All 72 unit tests, the production build/TypeScript check, full frontend formatting and final changed-file formatting/whitespace checks passed.
- T3 used tab-local read-only fixtures to confirm failed evidence recovery, preservation of a phone comment during a status outage, disabled submission while status was unknown, and restoration after the status recovered. No approval, comment submission or live backend change occurred. An attempted retry click targeted a notice already removed by successful background refresh; inspection confirmed the recovered state and retained draft, so no click was repeated. The final compiled phone controls were checked separately after the target-size adjustment.


- Eleventh iteration: all seven connection browser scenarios passed against the real Go ingress handlers and a synthetic control plane, with eight page/theme/width accessibility combinations and doubled-text overflow checks. Coverage includes missing/expired links, network/service/malformed errors, bounded waits, standalone versus embedded recovery, focus retention, same-tab fresh links, successful one-use exchange/redirect and replay rejection. The first run exposed the same-document fresh-link issue; the hash-change handler fixed it without automatic ticket retries. Browser checks run with `npm run test:connections` and leave live workers untouched.
- The dashboard Go tests, race detector and `go vet` passed. Frontend TypeScript and full formatting checks passed. T3 confirmed the original plain-text failures on the existing deployment, then the styled desktop preview and 320px worker recovery using an isolated fixture on port 5192. The fixture uses no live credentials or runtime. The main deployment has not been replaced; production and assistive-technology acceptance remain open.


- Tenth iteration: all 22 Work browser scenarios passed against a fresh production build, including three new checks for a 200-thread catalog, changed-digest title refresh and a conversation growing from 40 to 63 recorded events. The initial history test reproduced a real false New activity notice after older messages loaded; it passed after the sequence-based correction. The fixture verifies preserved reading position and keyboard scrolling, not multi-hour runtime endurance.
- The expanded populated-thread accessibility check found a real visible-label/accessibility-name mismatch. Removing the abbreviated ARIA label preserves the full visible title/status/date in the button name. After that correction and the phone target-size/count wording changes, all four affected title/search/draft-switching tests passed again, including four desktop/phone/theme axe combinations. No accessibility rule or assertion was suppressed.
- T3 inspection used a read-only, tab-local 200-thread fixture: 20 initial title reads, four concurrent reads at peak, complete search coverage, clear partial-result guidance, and successful retry to show both matching threads. The 320px drawer kept its search, recovery and footer controls readable. No backend records or worker state were changed. All 72 unit tests and the full frontend formatting check passed; the final production build and TypeScript check passed with the pre-existing chart-chunk size warning.

- Baseline verification pass before iteration ten: corrected formatting in the chart component and Work documentation so the full frontend formatting command passes. Updated the shared project-scope error to describe the selected project/worker rather than incorrectly referring to “both workers” in single-worker history and other scoped reads. Scope rejection behavior is unchanged. T3 inspection reconfirmed the current Projects, Platform services and phone Workers layouts with live read data; no live lifecycle changes were made. All 113 browser tests passed in six sequential production-build batches, with zero failures, skips or flaky results. The six saved JSON reports were reconciled against a fresh test catalog: 113 unique tests, none missing or duplicated. Aggregate evidence is `frontend/artifacts/full-suite/summary.json`. This was not a single uninterrupted invocation. The project-scope wording correction occurred after batch 3; scoped conversation checks passed in batch 5 and all 72 unit tests passed after the correction. The full formatting check and final TypeScript/build passed.

- Ninth iteration: all 17 final recovery/project-dialog browser tests passed against the production build and CSP. Coverage includes four page-module-failure/theme/width combinations with automated accessibility checks, enlarged-text fallback layouts, successful reload recovery, first-load project failures, independent Help recovery, sidebar retry, and preservation of a phone work draft with a visible retry action. All 14 browser-title checks and desktop/phone keyboard navigation passed in the preceding 15-test run. All 72 unit tests, build/TypeScript, formatting and whitespace checks passed.
- T3 inspection confirmed the missing-page and application-failure cards, recovery through Reload page, and a compact project-list outage warning with preserved draft and accessible support details. A retry removed the warning without losing the draft. Failures and draft content were simulated only inside the review tab; no live work was submitted or backend data changed.
- The initial draft-preservation test did not trigger a refresh because its synthetic visibility event was dispatched on document without bubbling. It was corrected to dispatch on window, matching the installed Query focus listener. The application assertion was retained and passed.

- Eighth iteration: a 17-test combined run passed 16 tests: all 56 page/theme/width accessibility combinations, all 28 text-spacing combinations, all 28 doubled-base-font page combinations, all 14 page titles, eight Hermes checks and four project/dialog checks. The keyboard test failed before its first interaction because Chromium aborted a lazy Projects module request (`net::ERR_ABORTED`); the bundle existed on disk. Cause remains unconfirmed; trace retained at `frontend/artifacts/ux-import-abort.zip`. The final title, keyboard, both-tab accessibility and enlarged-text checks then all passed against a fresh production build (4/4). After shortening the Hermes actions, all eight Hermes tests and the full 28-case enlarged-base-font check passed again (9/9). The latter also checks every visible Hermes button for horizontal text clipping in both tabs. The separate assistant-start test also passed, verifying a single backend workflow. After the final header adjustment, keyboard navigation and all 28 enlarged-base-font page cases passed again; T3 confirmed the compact phone header and readable setup actions. No assertion was suppressed.

- All 72 unit tests passed in the eighth iteration. Production build, TypeScript, changed-file formatting and whitespace checks passed. T3 confirmed keyboard activation moves focus into Getting started, page titles update, and help/Hermes controls reflow at 320px with a doubled base font. These checks cover responsive reflow and specific keyboard flows; they do not replace screen-reader or first-time-user acceptance testing.

- Seventh iteration: 13 focused browser tests passed history export success/failure/incomplete responses, cancelled export reads, history startup/pagination, log retries, partial home data, clipboard fallback and existing output/preview lifecycle behavior. Four populated history/log dialog-theme combinations passed automated accessibility checks at 320px. After the selector, log layout and shared hover corrections, all nine history/recovery browser tests passed again, including all four dialog/theme combinations and an explicit primary-action hover check. T3 inspection confirmed readable history selectors and log events at 390px using the compiled frontend and live read APIs.
- All 72 unit tests passed, including 10 export-envelope/error/cancellation checks. Production build, TypeScript, formatting and changed-file whitespace checks passed. No live export, pairing or lifecycle action was performed.
- Two early test failures used text locators that included additional task-ID content; they were corrected to inspect the rendered event. Renaming the output selector also exposed a test locator matching the Workers navigation link; tests now target its exact combobox role. No product assertions were disabled.


- Sixth iteration: the complete browser run passed 94/95 tests. Its only failure was a terminology mismatch: the desktop preview button still used “workspace” while the updated test expected “worker”. The button and history recovery action were aligned; the affected chart/desktop/mobile/keyboard test then passed against a fresh production build. All 95 browser scenarios have passed across the full run and that focused correction; this was not a single clean 95/95 invocation.
- Sixth iteration: all five new worker-dialog tests passed, including eight dialog/theme accessibility combinations at 320px. Initial scanning detected low contrast before the token correction. Existing smoke checks covered worker creation/branching, settings and file handoff; no accessibility rule was suppressed.
- T3 browser inspection confirmed the worker deletion dialog on desktop and creation on a phone layout, using the compiled production frontend and live read APIs. All dialogs were cancelled without live mutations. Unit tests (62/62), TypeScript build and changed-file formatting passed.


- Fifth iteration: the final 18-test focused browser run passed project creation, portfolio paging, deletion/profile dialogs and navigation regressions. Production build, TypeScript and changed-file formatting checks passed.
- Fifth iteration: the 14-page consistency suite passed all 56 page/theme/width axe-core combinations again. All 14 pages also passed increased line/letter/word/paragraph spacing checks at 1440px and 320px (28 combinations). These geometry checks do not establish complete text-resize or assistive-technology conformance.
- The initial combined run hit a Chromium renderer crash during an accessibility check. The subsequent full consistency run passed without changing or suppressing any accessibility assertions; the original trace is retained at `frontend/artifacts/ux-browser-crash.zip`. The cause was not established.
- T3 browser inspection confirmed project creation on desktop and at 320px, including doubled-text dialog wrapping and scrolling. Controlled browser fixtures verified a 120-project portfolio and pending/failure states without creating live projects. The pagination screenshot is `frontend/artifacts/project-pagination-phone.png`.


- Fourth iteration: all 9 Hermes/setup browser tests passed, including pending credential checks, cross-tab validation, failed-status recovery and profile retry without submission. Eight tab/theme/width accessibility combinations passed at 1440px and 320px. After the final Matrix status wording change, both affected recovery/accessibility tests passed again. The compiled production build and TypeScript checks passed; no accessibility rules were disabled.
- T3 browser inspection confirmed Agent desktop and Matrix phone layouts, including credential and connection controls. No live settings, credentials or Matrix connections were changed. An initial new test used an overly strict label locator for a native select; the semantic combobox locator passed with the existing accessible name.


- Third iteration: all 14 pages passed axe-core checks tagged WCAG A/AA through WCAG 2.2 in both dark and light themes at 1440px and 320px: **56 page/theme/width combinations**. Fixtures include long project, worker and profile names. This automated scan does not replace assistive-technology testing or establish complete WCAG conformance.
- Third-iteration focused runs covered 35 affected browser tests, verifying service outage recovery, repository confirmation/retry, existing work interactions, input limits, advanced-draft cancellation, and read retry without form submission. Navigation regressions passed. All 62 unit tests and the latest production build passed.
- Contrast scanning initially caught real failures in operator initials and destructive controls. Later transient-color findings were addressed by measuring after active CSS transitions finish; no accessibility rules were disabled. A broader animation wait stalled on activity animation and was replaced with a bounded transition-only check.
- T3 inspection confirmed the revised service diagnostics, new-thread and advanced-draft layouts, light-theme destructive control, and phone help layout. No live work or provider changes were submitted.

- Final production build and all 62 unit tests passed.
- First full browser run: 56/61 passed. Findings: obsolete schedule navigation expectation; obsolete three-item Platform menu expectation; board search height 36px instead of 32px; two navigation timeouts while source was changing. Those five initial failures passed subsequent targeted reruns.
- Added regression tests for network recovery, support disclosure, saved-theme CSP compatibility and missing-page recovery. Extended whole-page consistency coverage to Board, Reviews and Repositories.
- The first full combined run passed 63/64. Trace investigation corrected the initial interpretation: the not-found snapshot belonged to a leaked smoke-test tab at the retired `/tool-profiles` route. The actual test page had an empty root and aborted Vite development-module requests. Switched the smoke test to Playwright’s managed page fixture, and the browser host to the compiled bundle under the production CSP. The complete 64-test suite then passed. No application routing patch was needed. Original trace: `frontend/artifacts/ux-route-failure.zip`.
- Second-iteration focused checks: 8 output/recovery/consistency tests passed; 6 board/consistency tests passed; 21 recovery/review/output/profile/consistency tests passed; 3 deletion/consistency tests passed. Final full regression: **74/74 browser tests and 62/62 unit tests passed** with the compiled production bundle and CSP. The initial final-run attempt was interrupted and had one outdated text assertion after the worker terminology change; that assertion was updated before the clean full run. The final icon/label/terminology adjustments passed all 7 focused board/profile/whole-page checks and a fresh production build.
- Production bundle: all 14 main routes rendered a page heading without document overflow or active error alerts at 1440px. Saved light theme persisted across reload under production CSP; no data fonts remain in its stylesheet.
- Earlier sampled light-theme contrast ratios (before the sixth-iteration secondary-text adjustment): muted text 4.74:1, primary button text 6.28:1, destructive text against canvas 4.77:1. These are sampled tokens, not a complete WCAG audit.
- Changed source/test files passed targeted Prettier verification. Production build passes with a pre-existing warning about the large lazy-loaded chart bundle.

## Completion audit — current worktree

| Requirement | Evidence | Status |
| --- | --- | --- |
| Review and score every main page | All 14 active main routes and both secondary connection screens have T3 inspection and a score above; inactive scheduling routes remain removed. | Covered; current scores remain provisional. |
| Consistent desktop/phone layouts and readable controls | Compiled-bundle checks cover both themes, 320/1440px widths, text spacing, doubled base text, dialogs and navigation. | Verified for the recorded scenarios. |
| Clear errors and recoverable actions | Controlled network, query, module, mutation and partial-data failures; preserved drafts; guarded lifecycle actions; scoped support details; T3 recovery checks. | Verified for the recorded scenarios, not every possible backend incident. |
| Frontend regression | The original full run was terminated with SIGTERM after 49 passing tests and no reported test failure. Its orphan test server was stopped; six subsequent batches passed all 113 baseline tests, reconciled against the catalog, with JSON reports under `frontend/artifacts/full-suite/`. Iteration ten adds three Work scenarios, iteration eleven has a separate seven-scenario connection suite, and iteration twelve adds four Reviews scenarios, and iteration thirteen adds a 120-task Reviews queue scenario; each affected suite is verified separately. | Baseline and focused follow-up evidence; not a single final-revision full run or live integration sign-off. |
| Real integration acceptance | Existing records include a successful retained-preview job on 10 November and KVM/controller checks on 12 November. `docs/firecracker.md` explicitly leaves live Matrix conversation testing and additional end-to-end gates unverified. | Partial historical evidence; no new integration sign-off. |
| Production release readiness | `docs/security.md` and `docs/k3s.md` list outstanding identity, containment, failure-recovery and restore acceptance. The dashboard UI is now deployed locally; broader infrastructure acceptance remains separate. | Separate from the accepted UI release; not established by this task. |
| Accepted UX threshold | The user accepted 9/10 and requested finishing and deployment. All 14 main routes and both connection screens score at least 9 in the recorded assessment. | Accepted design threshold met; deployment verification recorded below. |

## Production and demo gates still open

- Verify live provider/Matrix/repository authentication and failures in an authorized test environment.
- Verify accessibility with assistive technology, real keyboard-only task completion and long content at enlarged text sizes.
- Production readiness additionally requires deployment, security, backup/restore, operational and load checks. UI inspection alone does not establish those.

The original 10/10 target was superseded by the user’s explicit acceptance of 9/10 and request to finish and deploy. This acceptance does not establish the separate production infrastructure or live-provider gates above.


## Accepted release — 14 November 2026

The user explicitly accepted 9/10 and requested finishing and deployment. Further issue-import exploration was stopped without implementation changes. All scored pages meet or exceed the accepted threshold.

- Deployed dashboard: `http://localhost:8080/app/`, using the existing base and Firecracker Compose configuration. Current image: `sha256:0143ed44424c6d2816730a2e65912d516065bab9bb2cfc17b5fb1d42861fcd03`. Dashboard and all existing platform services are healthy. Control-plane, runtime-controller and PostgreSQL container IDs/start times remained unchanged.
- Rollback image retained as `agent-workbench-dashboard:before-ux-20261114`, pointing to `sha256:66a7fef1db0a2f46e967c0ecd1a143417c59f3d061b88e6fe5601fecc90c8e14`. No volumes, credentials, repository connections or worker lifecycle state were changed by release validation.
- `GOFLAGS='-p=2' make check` passed: Go command builds, race tests, formatting, frontend dependency installation, 72 unit tests, production build, Python checks and Go vet. The initial unrestricted run had a Go linker killed during parallel compilation; the lower-concurrency run completed successfully.
- Full browser invocation passed 118/121. The three failures were stale exact-name selectors after thread buttons began exposing visible status/date metadata. The affected file was corrected and rerun: ten scenarios passed; its remaining assertion used another old title-only selector. That assertion was corrected and the final focused scenario passed. All 121 unique catalog scenarios now have passing evidence, reconciled without missing entries. This is full coverage across runs, not a single uninterrupted green invocation. All seven dedicated connection-page scenarios also passed.
- The application code did not change during release validation. Test-only selector updates preserve the title, filtering, visibility, keyboard and theme assertions. Final formatting and whitespace checks passed.
- All 61 deployed frontend files match the tested local production output by SHA-256. T3 post-deployment checks visited all 14 main routes: each had the expected heading/title, no active error alert and no document overflow at desktop width. The deployed preview and worker missing-link pages rendered contextual recovery; the worker page also fit 320px without horizontal overflow. No live decisions or test work were submitted.
- Logs, the reconciled catalog and summary are saved under `frontend/artifacts/release-20261114/` (ignored local evidence). npm reported six low/moderate advisories in the shadcn tooling dependency chain, none high/critical. The dashboard image serves compiled assets and Go; CLI dependencies are not installed in the runtime image. No force-upgrade was applied as part of this UI release.

The accepted polish-and-deploy scope is complete. The separate live-provider, assistive-technology and infrastructure acceptance items above are not represented as production certification.
