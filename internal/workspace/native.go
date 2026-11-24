package workspace

import (
	"agent-workbench/internal/shared"
	"agent-workbench/internal/workspace/nativeagent"
	"agent-workbench/internal/workspace/setup"
	"agent-workbench/internal/workspacetools"
	"context"
	"time"
)

const workspacePreviewGuidance = `Workspace delivery

If this task delivers or changes a runnable web app, leave a working Web preview for review before finishing. Do this even when the request does not explicitly ask for a preview. Use the workspace terminal to check workbench-preview status and reuse an existing preview if it serves the current app. Start or replace a preview only when needed for this app.

Use workbench-preview start --directory <workspace-relative-directory> --port <port> --command '<server-command>' to run the app under the workspace supervisor. Choose the directory and command from the project. Pass the assigned port using $PORT where supported. For a static site with an index.html, serve the directory that contains it, for example:
workbench-preview start --directory app --port 4173 --command 'python3 -m http.server "$PORT" --bind 127.0.0.1'

Before handing off, check that workbench-preview status reports the service ready and request the app's entry page from inside the workspace. Check that the response is the intended app. If startup fails, investigate and retry where possible. Keep the supervised service running for review. Ordinary background shell processes can be cleaned up when the agent run ends.

Say where to review the app in Workspace → Preview → Web. If the preview could not be verified, report the blocker. Use previews for runnable web deliverables and explicit preview requests. For other work, deliver the requested files, results, or explanation in the thread.`

const workspaceDesktopGuidance = `Desktop app development

Review runnable GUI apps in Workspace → Preview → Desktop. Use the worker's Wayland desktop. For GUI testing, run workbench-preview status to check for an active desktop. If needed, run workbench-preview desktop to start a supervised session. Launch the app with workbench-desktop run <app> [args...] to use the current Wayland session and its XWayland display for X11 apps. Verify the GUI and give the launch command so the app can be reopened from the desktop terminal. If the worker cannot start a desktop, report the blocker and provide the available verification results. Use the desktop for GUI review and deliver other results in the thread.`

const workspaceAnalysisGuidance = `For uploaded or large data, use the worker-local analysis tools. Run workbench-analysis list, schema ID, or preview ID REQUEST.json to inspect CSV or Parquet. Preview requests accept limit, cursor, columns, sort, and filters. Run workbench-analysis path ID to resolve a dataset in the agent shell. Use /opt/workbench-analysis/bin/python with the duckdb package for SQL and Python processing. Materialize bounded chart aggregations or other derived results as Parquet outside the Git checkout, then run workbench-analysis register-result FILE. Use ingest FILE for a derived dataset. Keep raw data and intermediate results outside Git. Never copy bulk data into messages or render it using the inline chart format.
To publish a visualization, write a version 1.0 JSON file and run workbench-analysis register-artifact SPEC.json. The envelope has schema_version, artifact_id, type, title, data (source_type and reference), and visualization. Use source_type dataset or query_result and the ID returned by the tool. A table uses optional page_size, columns as [{"field":"name"}], sortable, and filterable.
For a chart, set chart_type to line, bar, scatter, histogram, or pie. Line, bar, scatter, and histogram use encoding.x and encoding.y objects with field, type, and optional label. Use temporal, quantitative, or nominal for x and quantitative for y. Scatter and histogram need a numeric x. Histogram rows contain bin positions and counts. Set encoding.series to a grouping column for separate series. For pie, use encoding {"category":"category_column","value":"total_column"} and materialize complete, nonnegative totals with one row per category. Colors are optional. Use visualization.style.category_colors with six-digit hex colors keyed by exact category for a pie or a single-series nominal bar, e.g. {"Black":"#768498","Cinnamon":"#d98d59"}. For a series-based chart, use style.series_colors keyed by the series value, or the y label or field for one series. Configure gridlines on axis charts with style.gridlines, e.g. {"x":false,"y":false}. Use a new artifact ID for a changed spec.
For geo, use geometry point, coordinate_system with type geographic (crs EPSG:4326), projected (crs EPSG code), or cartesian, encoding with x and y numeric field names and optional color field, and view with mode map or cartesian and optional auto_fit. For a geographic density heatmap, keep geometry point and set layer heatmap with view mode map and CRS EPSG:4326. Use the source observation coordinates directly for observation density, or set encoding.weight to a nonnegative numeric count column for aggregated observations. Set optional radius_px between 8 and 100. Omit encoding.color for a heatmap. For independently toggleable point or heatmap layers in one map, set encoding.group to the categorical column. Preserve the source categories and distinguish a ground-plane category from actual below-ground observations. Do not substitute a colored point layer for a heatmap.
For geographic point aggregation, set layer grid (cell_size_m), hexagon (radius_m), contour (cell_size_m and optional positive thresholds), or screen_grid (cell_size_px). All use SUM of unit weights or numeric encoding.weight. Contours need a cell size appropriate for the geographic extent. For shapes, use geographic EPSG:4326 with view mode map, set geometry path, polygon, or geojson, and set encoding.shape to a JSON text column. Paths are arrays of [longitude, latitude]. Polygons are arrays of closed rings. GeoJSON rows contain one Geometry or Feature with Point, MultiPoint, LineString, MultiLineString, Polygon, or MultiPolygon. Use compact geometry and prepare one feature per CSV or Parquet row. Omit point encodings and aggregation parameters for shapes.
Reference each registered artifact in a fenced Markdown block with language harbormaster-visualization and only the ID on the next line. Use the same fence in reports and replies. To revise a registered visualization, use workbench-analysis artifact ID to read its specification, keep its data reference when the data does not change, and register a new artifact ID. For publication on approval, commit the report, specs, SQL and Python scripts, and a dependency manifest recording dataset name, format, SHA-256, processing script and query references, schema version, and parameters. Link the manifest from the report. Use the existing repository Git workflow. Interactive data follows the worker lifetime.`

func dataScienceInstruction(instruction string) string {
	return "Data science work. Investigate the data and deliver findings, reproducible analysis, visualizations, or a Markdown report as requested. Use Harbormaster artifacts for visualization. Do not build a website, dashboard, or web preview for this task. If source data is missing, explain what input is needed.\n\n" + workspaceAnalysisGuidance + "\n\n" + instruction
}

func workInstruction(instruction, kind string, native bool) string {
	switch kind {
	case shared.WorkKindDataScience:
		return dataScienceInstruction(instruction)
	case shared.WorkKindDesktop:
		return workspaceDesktopGuidance + "\n\n" + instruction
	default:
		if native {
			return workspacePreviewGuidance + "\n\n" + workspaceAnalysisGuidance + "\n\n" + instruction
		}
		return instruction
	}
}

func (m *Manager) runNativeTask(ctx context.Context, provider, instruction, taskID, model, effort, workKind string) (result string, code string) {
	m.event(provider, "agent.started", taskID, "", "")
	defer func() {
		kind := "agent.completed"
		if code != "" {
			kind = "agent.failed"
		}
		m.event(provider, kind, taskID, "", shared.SanitizedTaskError(code))
	}()
	cfg := nativeagent.Config{HermesMCPTransport: m.cfg.HermesMCPTransport, TaskID: taskID, ReasoningEffort: effort, ToolPath: setup.Path(m.homePath()), ProjectID: m.cfg.ProjectID, WorkspaceID: m.cfg.ID, DataDir: m.cfg.DataDir, HomeDir: m.homePath(), AgentKind: m.cfg.AgentKind, Token: m.cfg.Token, InferenceURL: m.cfg.InferenceURL, CodexModel: m.cfg.CodexModel, ClaudeModel: m.cfg.ClaudeModel, HermesModel: m.cfg.HermesModel, EgressURL: m.cfg.EgressURL, ConnectionMode: m.cfg.ConnectionMode}
	if model != "" {
		if provider == "codex" {
			cfg.CodexModel = model
		}
		if provider == "claude" {
			cfg.ClaudeModel = model
		}
	}
	var desktops int
	m.db.QueryRow(`SELECT count(*) FROM workspace_applications a JOIN retained_services s ON s.id=a.service_id WHERE s.state='active' AND s.expires_at>? AND json_extract(a.configuration,'$.kind')='desktop'`, time.Now().Unix()).Scan(&desktops)
	if desktops > 0 || workKind == shared.WorkKindDesktop {
		cfg.DesktopEnv = workspacetools.DesktopEnvironment()
	}
	return nativeagent.Run(ctx, cfg, provider, workInstruction(instruction, workKind, true), &nativeEvents{m: m, source: provider, task: taskID}, &nativeEvents{m: m, source: provider, task: taskID})
}
