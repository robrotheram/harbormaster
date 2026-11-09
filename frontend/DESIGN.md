# Workbench dashboard rebuild

An operator console for project-separated agent environments. The main job is
opening the right workspace and understanding its state and connection access.

Tokens: canvas #f6f7fa, surface #ffffff, ink #181b25, graphite rail #171b26,
indigo action #4f46e5, muted slate #70778b. Teal supplements explicit status labels.
Bundled Geist for the interface; Geist Mono only for identifiers and ports.
Type scale: 12px metadata, 14px controls, 16px sections, 26px page titles.

Layout:
[232px navigation rail] [64px project/context header + theme]
[project selector     ] [Page title                   Create]
[project navigation   ] [Search/filter toolbar              ]
[global settings      ] [Workspace inventory table          ]
[local operator       ] [File handoff + boundary details    ]

Review: a generic metric-card landing page obscures the development task.
Begin with actual workspace rows and a clear T3 action. Use a different structure
for global policies and focused dialogs for forms. Avoid marketing, fake
telemetry, duplicated project banners and decorative graphs. Borders and spacing
supply structure rather than shadows on every element. States and connection
access have independent labels. Keep security limitations in the rail and editor
rather than a large repeated warning above every screen.

Mobile navigation uses a sheet; workspace and profile rows stack into compact
summaries with visible actions, while secondary wide tables scroll internally. Theme, focus, status contrast and reduced motion are supported.
Tailwind v4 owns tokens, layouts and recipes. Checked-in shadcn/ui primitives
supply controls, dialogs, menus and tables. TanStack routing/query and backend
contracts remain. No runtime CDN/font requests.
