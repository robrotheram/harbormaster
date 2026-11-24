"""Run one confined schema inspection or preview against an inherited file."""
import datetime
import decimal
import json
import math
import os
import resource
import sys

resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
resource.setrlimit(resource.RLIMIT_AS, (512 << 20, 512 << 20))
try:
    import duckdb
except ImportError:
    print('{"error":"ANALYSIS_ENGINE_UNAVAILABLE"}', flush=True)
    os._exit(0)

MAX_COLUMNS = 512
RESPONSE_BUDGET = 900 << 10
CELL_LIMIT = 4096
SOURCE = "/proc/self/fd/3"
ORDINAL = "__harbormaster_ordinal"


def identifier(value):
    return '"' + value.replace('"', '""') + '"'


def encode(value):
    return json.dumps(value, ensure_ascii=False, allow_nan=False, separators=(",", ":"))


def cell(value):
    if isinstance(value, float) and not math.isfinite(value):
        return None, False
    if isinstance(value, int) and abs(value) > 9007199254740991:
        return str(value), False
    if isinstance(value, decimal.Decimal):
        return str(value), False
    if isinstance(value, (datetime.date, datetime.time)):
        return value.isoformat(), False
    if value is None or isinstance(value, (bool, int, float)):
        return value, False
    if isinstance(value, bytes):
        return value[:CELL_LIMIT // 2].hex(), len(value) > CELL_LIMIT // 2
    if not isinstance(value, str):
        value = str(value)
    return value[:CELL_LIMIT], len(value) > CELL_LIMIT


def preview(db, source, columns, request):
    by_name = {column["name"]: column for column in columns}
    fields = request.get("columns") or list(by_name)
    filters = request.get("filters") or []
    sorts = request.get("sort") or []
    ordinal = ORDINAL
    while ordinal.lower() in {name.lower() for name in by_name}:
        ordinal += "_"
    if any(field not in by_name for field in fields) or any(
        entry["field"] not in by_name for entry in filters + sorts
    ):
        return {"error": "ANALYSIS_UNKNOWN_FIELD"}

    params = [SOURCE]
    predicates = []
    comparisons = {"eq": "=", "ne": "<>", "lt": "<", "lte": "<=", "gt": ">", "gte": ">="}
    for entry in filters:
        field = identifier(entry["field"])
        op = entry["op"]
        if op == "is_null":
            predicates.append(f"{field} IS NULL")
        elif op == "is_not_null":
            predicates.append(f"{field} IS NOT NULL")
        elif op == "contains":
            predicates.append(f"contains(CAST({field} AS VARCHAR), ?)")
            params.append(entry["value"])
        else:
            predicates.append(f"{field} {comparisons[op]} ?")
            params.append(entry["value"])

    # The source is immutable. With a single thread and insertion order preserved,
    # row_number() over () supplies a stable tie breaker for both file formats.
    position = identifier(ordinal)
    select = ", ".join(identifier(name) for name in fields)
    order = ", ".join(
        [f"{identifier(entry['field'])} {entry['direction']} NULLS LAST" for entry in sorts] + [position]
    )
    where = " WHERE " + " AND ".join(predicates) if predicates else ""
    limit = request["limit"]
    if request.get("sample") or sorts:
        data = f"(SELECT *, row_number() OVER () AS {position} FROM {source}){where}"
    else:
        # LIMIT can stop the scan early. A single reader preserves source order,
        # including through WHERE, without an explicit sort of the entire file.
        data = source + where

    if request.get("sample"):
        query = (f"SELECT {select} FROM (SELECT * FROM {data} "
                 f"USING SAMPLE reservoir({limit + 1} ROWS) REPEATABLE (42)) ORDER BY {position}")
    else:
        ordering = f" ORDER BY {order}" if sorts else ""
        query = f"SELECT {select} FROM {data}{ordering} LIMIT ? OFFSET ?"
        params.extend([limit + 1, request.get("offset", 0)])
    result = db.execute(query, params)
    rows = []
    more = False
    truncated = False
    sampled = False
    size = len(encode(columns).encode("utf-8"))
    while (row := result.fetchone()) is not None:
        if len(rows) == limit:
            more = True
            sampled = request.get("sample", False)
            break
        values, clipped = zip(*(cell(value) for value in row))
        row_size = len(encode(values).encode("utf-8")) + 1
        if size + row_size > RESPONSE_BUDGET:
            more = True
            truncated = True
            if not rows:
                return {"error": "ANALYSIS_ROW_TOO_WIDE"}
            if request.get("sample"):
                # The reservoir includes one extra row when it omitted source data.
                sampled = len(rows) + 1 + sum(1 for _ in iter(result.fetchone, None)) > limit
            break
        size += row_size
        truncated |= any(clipped)
        rows.append(values)
    return {"columns": [by_name[field] for field in fields], "rows": rows,
            "has_more": more, "sampled": sampled, "truncated": truncated}


def run(request):
    source = ("read_csv(?, max_line_size=1048576, sample_size=16384, buffer_size=4194304)"
              if request["format"] == "csv" else "read_parquet(?)")
    config = {
        "threads": 1,
        "memory_limit": "176MiB",
        "temp_directory": request["scratch"],
        "max_temp_directory_size": "256MiB",
        "autoinstall_known_extensions": False,
        "autoload_known_extensions": False,
        "enable_external_file_cache": False,
        "preserve_insertion_order": True,
    }
    with duckdb.connect(config=config) as db:
        db.execute("SET allowed_paths = ?", [[SOURCE]])
        db.execute("SET enable_external_access = false")
        if "preview" not in request:
            description = db.execute(f"DESCRIBE SELECT * FROM {source}", [SOURCE]).fetchmany(MAX_COLUMNS + 1)
            if not description or len(description) > MAX_COLUMNS or any(len(row[0].encode()) > 256 for row in description):
                return {"error": "ANALYSIS_SCHEMA_TOO_WIDE"}
            return {"columns": [{"name": row[0], "type": row[1]} for row in description]}
        return preview(db, source, request["columns"], request["preview"])


try:
    request = json.load(sys.stdin)
    print(encode(run(request)), flush=True)
except (duckdb.Error, ValueError, OverflowError, MemoryError):
    # Engine errors can contain file paths or values from the dataset.
    print(encode({"error": "ANALYSIS_QUERY_FAILED" if "preview" in request else "INVALID_DATASET"}), flush=True)

# Each invocation owns one connection. With the virtual-memory cap applied,
# avoid native default-connection teardown during interpreter shutdown.
os._exit(0)
