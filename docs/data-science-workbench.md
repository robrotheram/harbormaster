# Analyze data on a worker

## Upload and explore a dataset

1. Open a running worker and select **Datasets**.
2. Upload a CSV or Parquet file. The worker records its schema, logical ID, and SHA-256.
3. Select the dataset to explore its rows. Use the column headers to sort. Use **Filter column** to filter.
4. Start a work conversation on that worker and select **Research / Data Science** as the work profile. Include the dataset ID in your request. The task opens in **Analysis**. Use **Datasets** to upload, inspect, and preview data for that worker.

Datasets and derived results live alongside the worker's Git checkout. The browser reads bounded previews through the workspace gateway. An agent can use native DuckDB and Python inside the worker to process the source data. Worker deletion can remove datasets, result files, and registered artifacts.

## Explore and delete visualizations

Open **Analysis** in a data science task to browse the charts, maps, and tables the agent has registered on its worker. Results appear automatically, including visualizations the agent created for a report.

Select **New visualization** to describe a new result to the agent. To experiment with an existing result, select it and choose **Ask agent to edit**. The agent registers the revision as a separate visualization. Select the pencil beside the title to edit the title or description for your account.

To remove a result, select **Delete visualization** and confirm. The dataset and derived data stay on the worker. Reports and replies that reference the deleted visualization show it as unavailable.

## Build and export a report

Open **Report** in a data science task. When the agent updates the same report file, Workbench keeps one entry. Use the **Report version** menu to preview or export an earlier captured version.

1. Select **New report** or open an existing report and select **Edit**.
2. Set the title and write your findings. Use the formatting toolbar for headings, lists, links, and other Markdown formatting.
3. Find a table, chart, or map under **Visualizations**, select **Copy Markdown**, and paste the snippet on a blank line where the result should appear. You can paste a result more than once.
4. Select **Save and preview** to save the Markdown to the worker and review the report.
5. Open **Export** and choose **Download PDF**, **Download HTML**, or **Download Markdown**.

Select a visualization name in the editor to open that result in **Analysis**. Results labeled **Used in report** have an **Ask agent to edit in report** action. The agent registers the revision and updates the existing report.

HTML and PDF include up to 25 rows per referenced table and static views of the charts and maps. PDF uses an A4 layout with selectable text and numbered pages. The Markdown keeps figure references in `worker_id/artifact_id` form. Open the Markdown in the project's file viewer to render those references while the originating worker and its data are available.

## Analyze data in the agent shell

| Command | Result |
| --- | --- |
| `workbench-analysis list` | Metadata for the worker's datasets |
| `workbench-analysis schema ID` | Schema, format, size, and checksum for a dataset or result |
| `workbench-analysis path ID` | Local path for DuckDB or Python |
| `workbench-analysis preview ID REQUEST.json` | Bounded preview using the HTTP request format below |
| `workbench-analysis ingest FILE` | Logical ID for a derived CSV or Parquet dataset |
| `workbench-analysis register-result FILE` | Logical ID for a CSV or Parquet query result |
| `workbench-analysis artifact ID` | Registered visualization specification |
| `workbench-analysis register-artifact SPEC.json` | Validated artifact, or an error describing the invalid field or reference |

Use `/opt/workbench-analysis/bin/python` to import `duckdb` in the worker. Write derived data to a temporary directory outside the Git checkout. For example, create histogram input from a Parquet dataset with a named DuckDB relation.

```python
import sys
import duckdb

source, output = sys.argv[1:]
with duckdb.connect(config={"threads": 1}) as db:
	db.read_parquet(source).create_view("sensor")
	bins = db.sql("""
		SELECT floor(temperature / 5) * 5 AS bin, count(*) AS readings
		FROM sensor
		WHERE temperature IS NOT NULL
		GROUP BY bin
		ORDER BY bin
	""")
	bins.write_parquet(output)
```

Register the Parquet file with `workbench-analysis register-result`. Pass its result ID to the artifact spec.

## Define and reference a visualization

[Artifact schema 1.0](../internal/workspace/analysis/artifact-v1.schema.json) defines the allowed fields. Register the JSON specification in the worker before referencing its ID in a report or reply. IDs are immutable within the worker. Use a new artifact ID for a revised specification.

```json
{
	"schema_version": "1.0",
	"artifact_id": "temperature_histogram",
	"type": "chart",
	"title": "Temperature distribution",
	"data": {
		"source_type": "query_result",
		"reference": "result_REPLACE_WITH_REGISTERED_ID"
	},
	"visualization": {
		"chart_type": "histogram",
		"encoding": {
			"x": {"field": "bin", "type": "quantitative", "label": "Temperature bin"},
			"y": {"field": "readings", "type": "quantitative"}
		}
	}
}
```

| Type | Data and rendering |
| --- | --- |
| `table` | Optional column projection, page size, sorting, and filtering. Headers show DuckDB types. |
| `chart` | Supports line, bar, scatter, histogram, and pie. Use quantitative Y values on axis charts. X supports temporal, quantitative, and nominal fields. Scatter and histogram use numeric X. Histograms use precomputed, equal-width bins. Set `encoding.series` to group rows. |
| `geo` | deck.gl renders numeric XY points. Geographic coordinates use longitude and latitude in EPSG:4326 and a MapLibre basemap. Projected coordinates use planar axes in the declared EPSG CRS. Cartesian coordinates use planar axes in the dataset's units. |

Use a fenced block with language `harbormaster-visualization` and the artifact ID on its own line.

````markdown
## Temperature distribution

There were two readings in the highest temperature bin.

```harbormaster-visualization
temperature_histogram
```
````

The Markdown renderer resolves the ID against the worker attached to the attempt. If the spec or its data is missing, it displays an unavailable-data message. Geographic maps load streets and labels from OpenFreeMap in Web Mercator. The browser sends the displayed tile coordinates to OpenFreeMap when a geographic map opens. The map falls back to the bundled Natural Earth land layer if the provider is unavailable. [OpenFreeMap quick start](https://openfreemap.org/quick_start/) Points beyond the Mercator latitude limit are omitted from the map. [MapLibre integration in deck.gl](https://deck.gl/docs/api-reference/maplibre/overview)

## Set chart colors and gridlines

Use `visualization.style.category_colors` to set category colors on a pie or an ungrouped bar chart with nominal X values. The keys match the category values. For grouped or numeric axis charts, use `style.series_colors` keyed by series value. For a single series, use the Y label or field name as the key. Use six-digit hex colors such as `#d98d59`.

Set `style.gridlines.x` and `style.gridlines.y` to control the gridlines on axis charts. By default, the Y axis draws gridlines.

For a pie, aggregate the data to one row per category with a nonnegative total. Set `chart_type` to `pie` and supply `encoding.category` and `encoding.value` as column names. The pie needs complete totals for its percentages.

## Map observation density

Set `geometry` to `point` and `layer` to `heatmap` on a geographic `geo` artifact. Use EPSG:4326 longitude and latitude. With no weight field, every observation contributes equally. For aggregated observations, provide a numeric count field in `encoding.weight`. Rows with nonpositive or missing weights are skipped. The optional `radius_px` sets smoothing from 8 to 100 screen pixels and defaults to 35. Colors represent relative density for the current view and can change with zoom. The standard geo data limit and sampling indicator also apply. [deck.gl HeatmapLayer](https://deck.gl/docs/api-reference/aggregation-layers/heatmap-layer)

## Toggle observation groups

Set `encoding.group` to a grouping column on a point or heatmap artifact. Grouped heatmaps use a distinct color and scale density separately per layer. The map shows a labeled checkbox for each displayed group. Existing point maps with a text or boolean color column use that column for the controls. The map groups small categories into **Other categories** when there are more than 12. Toggles preserve the current map view.

## Use additional geographic layers

Use EPSG:4326 and map view for these layers. Aggregation layers accept numeric longitude and latitude with an optional numeric weight field.

| Layer | Settings and behavior |
| --- | --- |
| `grid` | Sum weights in square cells. `cell_size_m` defaults to 250 metres. |
| `hexagon` | Sum weights in hexagons. `radius_m` defaults to 250 metres. |
| `contour` | Draw lines at summed weights from `thresholds`, which defaults to `[1, 5, 10]`. `cell_size_m` defaults to 250 metres. Use a coarser grid for large geographic extents. |
| `screen_grid` | Sum weights in cells measured in screen pixels. `cell_size_px` defaults to 32 pixels. Bins change with the view. |

For shape layers, set `geometry` to `path`, `polygon`, or `geojson` and set `encoding.shape` to a JSON text column. Each row must contain one shape.

- A path is an array of at least two longitude and latitude pairs.
- A polygon is an array of closed rings. Put the outer ring first, followed by any holes.
- A GeoJSON row contains one Geometry or one Feature with a Point, MultiPoint, LineString, MultiLineString, Polygon, or MultiPolygon. Split FeatureCollections into separate rows before registering them.

The renderer excludes invalid shapes and caps the total at 20,000 vertices per artifact. Split or simplify long features to fit the existing data cell limit of 4,096 characters. [deck.gl layer catalog](https://deck.gl/docs/api-reference/layers)

## Publish a report

Use the work conversation to review and refine the report. After approval, commit the Markdown, artifact specs, SQL or Python scripts, and a dataset dependency manifest through the project's existing Git workflow. Include each dataset's name, format, SHA-256, processing script and query references, visualization schema version, and processing parameters in the manifest. Link the manifest from the report. Keep raw and derived data outside the checkout.

After the worker is deleted, retrieve committed reports, specs, and processing code from the Git remote. To restore an interactive result, upload the original dataset to the target worker, verify the checksum, rerun the analysis, and register the spec with the new logical data ID.

## Workspace API

The authenticated control-plane prefix is `/v1/workspaces/{id}/analysis`. The dashboard uses `/ui-api/workspaces/{id}/analysis`. Access checks run before dispatch to the worker. Upload, preview, and spec routes use the existing controller grant. Result and artifact registration are local agent operations.

| Method | Path | Body |
| --- | --- | --- |
| GET | `/datasets` | |
| POST | `/datasets/upload?name=sensors.parquet` | File bytes as `application/octet-stream` |
| GET | `/datasets/{dataset}/schema` | |
| POST | `/datasets/{dataset}/preview` | Preview request |
| GET | `/artifacts` | |
| GET | `/artifacts/{artifact}` | |
| POST | `/artifacts/{artifact}/data` | Table page controls or an empty object for a chart or geo artifact |
| POST | `/artifacts/{artifact}/delete` | Empty object |

Browser POSTs require the existing `X-Workbench-Request: 1` header. Include the current `project_id` when calling the dashboard API. Artifact and dataset IDs are resolved only in the authorized worker.

Use `GET /v1/projects/{id}/analysis/visualization-details` to list the current user's title and description edits. Send a POST to the same path with `worker_id`, `artifact_id`, and the fields to change. Send an empty description to clear it.

A preview request accepts `limit`, `cursor`, `columns`, `sort`, and `filters`. Sort entries contain `field` and `direction`. Filter entries contain `field`, `op`, and, when required, a scalar `value`. Operators are `eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `contains`, `is_null`, and `is_not_null`. Multiple filters use AND. Use strings for exact integers beyond JavaScript's safe integer range.

The response contains `columns`, `rows`, `next_cursor`, `has_more`, `sampled`, and `truncated`. Pass the returned cursor with the same request to read another page. Changing the filters, sort order, projection, or limit starts a new query. Ordered pages use the source row position as a tie breaker. Later sorted pages can require rescanning the source.

A worker accepts one upload and runs one DuckDB query at a time. It queues at most three additional queries within the query deadline. Uploads have a 1 GiB file limit and a 10-minute deadline. Datasets and results have a combined 4 GiB limit and a 128-file limit. Queries have an 8-second deadline and a 1 MiB response cap. Tables return up to 500 rows. Charts return up to 2,000 rows. Geo artifacts return up to 5,000 points. Scatter and geo use a seeded reservoir sample for larger data. Table responses can return fewer rows when the byte cap applies. Queries that exceed the DuckDB memory or spill budget return an error. Dataset metadata uses native DuckDB type names.

## Extend the renderers

For a new visualization kind or encoding, update the versioned schema, the worker's semantic validation and data projection, and the frontend's typed renderer. Keep browser requests bounded and resolve logical sources in `internal/workspace/analysis`. The frontend renderer owns library-specific options in `frontend/src/features/analysis`.

See the [repository integration plan](data-science-workbench-plan.md) for the gateway, worker lifecycle, and task extensions.
