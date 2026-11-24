# Software factory

> Historical record. See the [documentation index](../docs/README.md) for current setup and workflows.

The product is a compact operator console for an AI software factory. Projects
own the work and Hermes context; global infrastructure shows worker observations.
T3 Desktop is the operator client, the control plane dispatches work, and separate
worker sandboxes execute it. Never imply a runtime or connection is verified when
its backend observation says otherwise.

## Design plan

Preserve the existing neutral/blue palette: canvas #0a0a0a, rail #141414,
surface #171717, rule #292929, text #fafafa, action #193cb8 (existing tokens).
Geist is the interface face; JetBrains Mono is reserved for IDs and resource data.
Use 11–12px metadata, 13px body/controls and navigation, 16px sections,
and 20px page titles.
Dark remains the default with a working light preference.

Layout, left aligned:

    [Workbench      ] [Project / current section                   ]
    [Project picker ] [Threads (Work only) ] [Conversation          ]
    [Work           ] [Search / new       ] [                      ]
    [Workers        ] [Thread list        ] [Messages    | Preview ]
    [Outputs / …    ] [                   ] [Composer              ]
    [Factory links  ] [Other sections use the whole content area   ]

Global infrastructure opens directly on resource allocations and a filterable
cross-project worker inventory with selection and bulk lifecycle actions. Platform
services is available as Health in the sidebar’s System menu. The execution
architecture diagram, redundant header scope and page footer have been removed.
Bulk operations is an explicit mode: its action bar and compact selection column
appear only on request, keeping worker names and normal browsing free of controls.
Sidebar and content headers share one 54px height in both sidebar states; logo,
project context aligns to the same horizontal guides. The sidebar collapse control
belongs in its own brand row, not in the main header.
The sidebar is 228px wide (64px collapsed), with 40px navigation targets and
an explicitly labelled New project action. Every page uses the same project-first
sidebar: a project switcher, vertical project sections, then secondary factory
navigation. Visiting Projects, Infrastructure, connections or help does not reorder
or replace the navigation. The sidebar retains the last valid project; only the
current route is highlighted. The header shows project and section
context without horizontal tabs or any reserved thread-column width. Only Work
adds a persistent thread panel on desktop; there is no thread collapse control.
Projects are a compact list with actual work stage counts and links into delivery.

## Brief review

A generic overview with large KPI cards would obscure the factory's purpose.
The primary surfaces are work and worker routing. The project companion is useful
because it starts or runs the real Hermes workspace; it is not a fake chat feed.
Project section links are grouped beneath the selected project in the sidebar.
Workspace lifecycle, previews, pairing and settings remain reachable.
No fabricated activity, progress percentages, capacity graphs, or connection states.

## Review bar

Score hierarchy, factory comprehension, density, consistency, responsive behavior,
accessibility, and functional honesty. Review rendered desktop and mobile screens,
iterate on specific weaknesses, and exercise project switching and work actions.
A visual score is a design judgment, not evidence of demand or investment interest.

## Final visual review

The earlier 9/10 assessment missed a sidebar/header alignment defect and was
withdrawn. Review now includes measured header boundaries and icon centers in
both expanded and collapsed states, alongside rendered screenshot inspection.

Rendered review fixes: corrected search-icon padding, removed mobile document
overflow, made work and infrastructure rows readable without horizontal scrolling
on phones, separated accepted/review status colors, surfaced a direct review
action, simplified work names while preserving their actual specification paths,
and added a mobile jump to Hermes. Dark and light themes were inspected.

Verification includes production build, formatting, unit tests and browser
contracts. Responsive checks cover 320, 390, 768, 1024, 1280 and 1440px widths.
Visual fixtures live only in tests; product data comes from the existing APIs.
Screenshots in ignored `artifacts/factory-*.png` document the reviewed surfaces.
This frontend change does not provision a Kata deployment or establish live T3
or provider acceptance; it exposes the existing workflows and runtime observations.

## Whole-app consistency audit

Reviewed all 13 main pages in the deployed app, including project work, workers,
outputs, schedules, runs, homes, Hermes setup and settings, plus the five global
pages. The native-select utility layer was overriding component-level sizing:
compact filters now opt into the shared 32px size explicitly. Infrastructure uses
a 20px leading gutter aligned with its heading, filters and row count. Bulk
selection remains an explicit mode.

Outputs now has a matching page title and truthful loading/empty states. Run names
and workers occupy separate lines. Runs and homes use labelled mobile rows, and
Hermes form columns can shrink without overflowing. Extended operational notes
are expandable; they no longer dominate the content. The Workers heading matches
its navigation label. Search fields share the same size throughout the app.

`tests/ui-consistency.spec.ts` covers all main routes at 1440px and 320px, checking
page headings, document overflow, search/filter heights and worker-name spacing.
It complements the existing workflow, lifecycle, keyboard and theme tests. Scores
are design judgments; production task completion and customer value require their
own evidence and are not inferred from screenshots or mocked workflows.

Workers now share the compact inventory treatment: 20px leading gutter, 32px
filter controls, two-line identities and allocation-first resources. Runtime
observations that do not exist on stopped workers no longer dominate their rows.
Project context appears once in the header; switching lives in the sidebar.
A startup review turns failed starts into a specific diagnosis with a next action.
Reviewing startup is read-only, and the final Start worker action stays disabled
until the checks pass. Project utilities live in a single menu above the inventory.

Hermes setup fills the available workspace width with Agent and Matrix tabs.
Its header, status strip and form align to the same 24px desktop gutters,
including on wide displays and with the sidebar collapsed. No nested width cap
or secondary column narrows the form. Runtime,
worker and Matrix status share a compact strip above the form. Matrix room
settings, credential replacement, and connection actions stay in the same panel;
stored credentials and extended help are collapsed. Keyboard tab navigation and
mobile layouts preserve the form draft. A running agent opens its workspace;
a connected Matrix bot shows its disconnect action without a redundant Connect.

## Conversation-first work (current direction)

The developer/researcher works in one persistent conversation. The central
reading column holds the user's brief, recorded agent messages, expandable tool
calls and results, and a composer. A narrow left list keeps other work reachable;
an adjacent output panel holds the selected run's files and acceptance action.
Starting work and sending revisions stay in this surface. Infrastructure and
worker setup remain secondary tools.

Preserve Geist, the existing light/dark neutral surfaces, blue #193cb8 actions,
#292929 dark dividers, #fafafa foreground, and #141414 secondary surfaces. Body
conversation text is 14px with comfortable line height and a roughly 75-character
measure; controls and tracking metadata stay compact. All columns align left.

    [Work list] [Brief → agent messages → tool activity] [Outputs / review]
                [Message composer, run / stop controls]

The strongest structural choice is a continuous conversation with tools revealed
in context, rather than separate submission and review forms. Avoid a floating
chat bubble aesthetic: this is a working record for code, commands and research.
Show only recorded events, mark unavailable or partial history, and preserve
scroll position while polling. Sending a follow-up starts a new durable run with
the prior instructions and workspace files; it does not imply native session
resume or support for injecting messages into an active run. Small screens use
one main reading column, with work selection and outputs readily expandable.

Conversation review: inspected the rendered 1440px and phone layouts and checked
320/390/1024/1440px overflow, task/project isolation, failed-send recovery,
cancellation, output escaping and acceptance after reload. Screenshot captures
disable transitions so the sidebar's resize animation cannot obscure content.
Live acceptance through the deployed authenticated ingress created a native run,
continued it with a second instruction, verified both independently retained file
versions, and persisted acceptance. Active event polling keeps observed events and
reads subsequent ascending pages so a burst cannot displace earlier tool activity.

Live preview extends the conversation's output column with a compact Preview
section: application directory, start command and port, followed by observed
readiness, Open preview and Stop controls. Preserve existing Geist, #141414
secondary surfaces, #292929 dividers, #fafafa text and #193cb8 actions. Logs stay
collapsed until needed. The application opens on an isolated preview origin;
worker configuration stays beside the conversation instead of another page.

## T3-inspired working view

Make the conversation and its visual result the two primary surfaces. Keep Geist,
#0a0a0a canvas, #141414 panels, #292929 dividers, #fafafa text and #193cb8 actions.
A single viewport-height split replaces the scrolling stack of Outputs, review,
preview forms and diagnostics. The user can resize or hide the visual pane; on
phones, Conversation and Preview are separate modes. The work list becomes a
compact drawer while a conversation is open. Show only the latest run by default;
older attempts and individual tool details are available on demand.

    [Conversation title / status / History]
    [Messages and grouped activity] | [Web | Desktop | Visuals    Files]
    [Message composer             ] | [Rendered content       ]

The pane toolbar owns app selection, refresh, viewport size, selection-to-feedback,
and configuration. Setup and logs live in dialogs. Acceptance is a small action
near the latest reply. Visualizations render retained data. Desktop mode opens
the worker's Wayland session, which supports X11 apps through XWayland.

Saved files have one quiet entry at the right of the preview pane header. Remove
per-run output links and the conversation-header Files button; run selection stays
inside the files dialog. This keeps output navigation with the visual tools.

Application logs open from a terminal icon into a near-full-window debugging view.
Use 13px Geist Mono, generous line height, and one scrollable log surface. Keep
command and directory visible; filtering, pause, follow, wrapping and export are
controls inside this view, so the main conversation stays quiet.


## Conversation-led results

Coder Agents presents enterprise deployment and governance as its lead proposition
(https://coder.com/solutions/agents). Our intended distinction is the daily work
experience: a continuous conversation joining instructions, observable execution,
software results, research experiments, retained evidence and human review.
This is a product direction, not a claim that competitors lack those features.

Charts belong with the conversation’s results. Remove the separate Visuals tab.
Preview is a collapsible supporting surface for Web and Desktop. Detect actual
HTTP listeners inside the worker and forward them without asking the user to
translate a localhost address or re-enter the agent’s launch command. Keep file
inspection, error diagnosis and change requests in the same work item. Infrastructure
controls should remain secondary to the user’s task and its evidence.

The working page is capped at 1560px wide and 840px tall on large displays. Align
message content and the composer on a shared 760px reading column; avoid a full-width
composer beneath narrow messages. Preview keeps at least 280px while resizing.
Short screens shrink the workspace instead of forcing the composer below the fold.

User correction: use the full available width. Remove the 1560px page cap and the
760px reading-column padding from messages and composer. App launch belongs to
the agent inside the workspace, using supervised processes; preview forwards them.

## Quieter conversation, real visual results

Keep the established Geist type and palette (#0a0a0a canvas, #141414 panels,
#292929 dividers, #fafafa text, #a1a1aa secondary text, #193cb8 actions).
Use the full available width with a bounded split that cannot overflow. The agent's
reply and generated image are the focal point. Render Markdown and workspace images
in the reply itself; never show an empty results card. Put acceptance in the header,
files and chart tools in a menu, and keep the composer to message plus send. Preview
keeps refresh and collapse visible, with secondary tools in a menu. Refresh web
content after a completed run; a revoked app clears its frame and stale errors.

    [Conversation / review / more] | [Web / Desktop / refresh / more / hide]
    [Agent reply + actual image]  | [Running application]
    [Message …             Send]  | [                   ]

Retain the user's full-width choice rather than reintroducing a narrow reading
column. Charts remain inline once they have data; manual data entry opens a dialog.

## Browser-rendered exploration

Use the same Geist typography and existing neutral canvas/panel/text tokens. Charts
carry the color: #5e82f7 blue, #34b99b teal, #c18d50 ochre, #a389d4 violet. Keep
one chart title and a compact Explore / Data / Export row. Put axes, metrics and
filters behind Explore. The plot is the focal point, with hover values, zoom and
legend toggles. Selecting a point offers a contextual follow-up in the existing
composer. All chart drawing happens in the browser from bounded, validated data;
agent output supplies measurements and a small declarative chart specification.
No additional preview tab, empty chart panel, or per-chart infrastructure controls.

    [Disk usage                           Explore | Data | Export]
    [                   interactive chart                       ]
    [Selected: outputs · 12 KiB             Ask agent about this]

### Work workspace: navigation and reading hierarchy

The user chose project sections in the left sidebar, with threads opening beside
Work. Do not couple application navigation to Work's thread-panel width. Within a
project the sidebar has a project switcher, the section links, then secondary
factory links. This same structure stays mounted visually on all global pages.
The existing project index is still available from Projects.
There is no horizontal project-tab row.

Threads exist only within Work. The desktop panel stays visible and owns search,
filtering and new-thread creation. Do not add a thread collapse control. The status
filter uses a themed radio menu rather than native option popups; text and surface
contrast must remain readable in both themes. Below 1200px, the thread drawer remains reachable from conversation
and preview. Mobile project navigation uses the separate application drawer.
Switching projects retains the selected project section and clears private views.

Keep the neutral/blue tokens and Geist typography. Use shared pane boundaries
inside Work, independent list and message scrolling, and a pinned composer. The
project header has a consistent 54px height across sections, with no Work-specific
layout dependency. Thread and conversation heading rows use a 72px minimum.

Review the complete shell in expanded and collapsed states, Work with its persistent
thread panel, Workers and Settings, mobile drawers, project switching, and both
themes. Verify that other sections have no thread controls or reserved space and
that navigating between sections does not move the project navigation.


### Stable project-first navigation

Remember the last selected project ID in browser preferences and resolve it against
the current accessible project list. The route's project takes precedence; if the
remembered project is absent, use the first available project. Global pages keep
that context available without selecting a project section or redirecting away
from the global page. Choosing a project from a global page opens its Work view.

While loading or with no projects, keep the same switcher and section slots. The
empty state offers New project and disables project-only destinations. Factory
links keep their position and remain usable. Verify the positions of the switcher,
project sections and global links across project, Projects, Infrastructure,
services and connections pages, in expanded and collapsed sidebar states.


### System tools and documentation

Keep the lower sidebar compact: Projects plus one System menu. System contains
Infrastructure, Connection profiles and Health (the existing Platform services
page). The menu trigger stays in the same position on every route and indicates
when a system page is active. Its destinations remain accessible in the collapsed
sidebar and mobile navigation drawer.

Getting started is linked as Docs in the top-right header beside the theme toggle.
Use the book icon and visible Docs label on wider screens, and a labeled 44px icon
target on phones. Remove the separate sidebar help link and Health section. The
project switcher, section navigation and remembered project behavior stay intact.

Health uses a heart-pulse icon and an observed-status badge in the System menu.
Refresh service observations while the menu is open. Show Healthy only when all
reported services are healthy; a known failure yields Degraded. Missing, incomplete
or failed observations remain Unknown, with Checking during the initial fetch.

The main sidebar's collapse/expand button lives in its top brand row beside
Workbench. In the compact sidebar this row shows the expand control alone. On short viewports,
project sections scroll within the sidebar so footer controls remain reachable.
Mobile retains the header navigation button and the Work thread drawer.

## Issue board

The board now uses four planning lanes (Planned, In progress, Review, Done), compact draggable cards, amber attention highlights and a separate archive view. Card details expose explicit repository title/status sync and its conflicts. See [board design and behaviour](../docs/issue-board.md).

Reviews use a 260px delivery queue and a large inspection surface with UI preview,
Code review, Visualisations and Reports tabs. Keep live workspace content visibly
separate from recorded attempt evidence. Historical review badges use the stored
decision, not just the worker execution state. Decisions stay beside the work: Approve, or a comment and Request changes.
Preview startup uses the application service and never creates a work revision.
