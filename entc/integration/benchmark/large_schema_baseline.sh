#!/usr/bin/env bash

set -euo pipefail

export LC_ALL=C

ROOT="$(git rev-parse --show-toplevel)"
RUNS="${RUNS:-3}"

if ! [[ "$RUNS" =~ ^[0-9]+$ ]] || (( RUNS < 1 )); then
	echo "RUNS must be a positive integer, got: $RUNS" >&2
	exit 1
fi

DEFAULT_OUTPUT="entc/integration/benchmark/artifacts/large-schema-baseline.json"
OUTPUT_PATH="${1:-$DEFAULT_OUTPUT}"
if [[ "$OUTPUT_PATH" != /* ]]; then
	OUTPUT_PATH="$ROOT/$OUTPUT_PATH"
fi

MODULE_DIR="entc/integration"
FIXTURE_PATH="entc/integration/ent/schema"
PACKAGE_PATH="entc/integration/ent"
CODEGEN_CMD="${CODEGEN_CMD:-go generate ./ent}"
BUILD_CMD="${BUILD_CMD:-go build ./ent}"

TMP_DIR="$(mktemp -d)"
cleanup() {
	chmod -R u+w "$TMP_DIR" 2>/dev/null || true
	rm -rf "$TMP_DIR"
}
trap cleanup EXIT
GOMODCACHE_DIR="$TMP_DIR/gomodcache"
GOPATH_DIR="$TMP_DIR/gopath"
mkdir -p "$GOMODCACHE_DIR"
mkdir -p "$GOPATH_DIR"

run_stage() {
	local stage="$1"
	local cmd="$2"
	local csv="$TMP_DIR/$stage.csv"
	: >"$csv"
	for run in $(seq 1 "$RUNS"); do
		local metrics_file="$TMP_DIR/${stage}-${run}.metrics"
		local run_log="$TMP_DIR/${stage}-${run}.log"
		local gocache="$TMP_DIR/gocache-${stage}-${run}"
		mkdir -p "$gocache"
		if ! (
			cd "$ROOT/$MODULE_DIR"
			/usr/bin/time -f "%e,%M" -o "$metrics_file" env CGO_ENABLED=0 GOFLAGS="-modcacherw" GOPATH="$GOPATH_DIR" GOCACHE="$gocache" GOMODCACHE="$GOMODCACHE_DIR" bash -c "$cmd" >"$run_log" 2>&1
		); then
			echo "benchmark command failed for stage '$stage' run $run: $cmd" >&2
			cat "$run_log" >&2
			exit 1
		fi
		local wall rss
		IFS=, read -r wall rss <"$metrics_file"
		printf "%s,%s,%s\n" "$run" "$wall" "$rss" >>"$csv"
	done
}

samples_json() {
	local csv="$1"
	awk -F, 'BEGIN{first=1} {if(!first)printf(","); first=0; printf("{\"run\":%d,\"wall_seconds\":%s,\"peak_rss_kb\":%s}", $1, $2, $3)}' "$csv"
}

metric_stats_json() {
	local csv="$1"
	local field="$2"
	local sorted="$TMP_DIR/$(basename "$csv").${field}.sorted"
	awk -F, -v field="$field" '{print $field}' "$csv" | sort -n >"$sorted"
	local min max median
	min="$(head -n1 "$sorted")"
	max="$(tail -n1 "$sorted")"
	median="$(
		awk '
			{vals[NR]=$1}
			END {
				if (NR % 2 == 1) {
					print vals[(NR+1)/2]
				} else {
					printf "%.6f", (vals[NR/2] + vals[NR/2+1]) / 2
				}
			}
		' "$sorted"
	)"
	printf '{"min":%s,"median":%s,"max":%s}' "$min" "$median" "$max"
}

stage_json() {
	local cmd="$1"
	local csv="$2"
	local wall_stats rss_stats samples
	wall_stats="$(metric_stats_json "$csv" 2)"
	rss_stats="$(metric_stats_json "$csv" 3)"
	samples="$(samples_json "$csv")"
	printf '{"command":"%s","samples":[%s],"wall_seconds":%s,"peak_rss_kb":%s}' "$cmd" "$samples" "$wall_stats" "$rss_stats"
}

mkdir -p "$(dirname "$OUTPUT_PATH")"

run_stage "codegen" "$CODEGEN_CMD"
run_stage "build" "$BUILD_CMD"

timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cat >"$OUTPUT_PATH" <<EOF
{
  "name": "entc-large-schema-baseline",
  "timestamp_utc": "$timestamp",
  "fixture": "$FIXTURE_PATH",
  "module_dir": "$MODULE_DIR",
  "package": "$PACKAGE_PATH",
  "runs": $RUNS,
  "commands": {
    "codegen": $(stage_json "$CODEGEN_CMD" "$TMP_DIR/codegen.csv"),
    "build": $(stage_json "$BUILD_CMD" "$TMP_DIR/build.csv")
  }
}
EOF

echo "Wrote baseline benchmark artifact to $OUTPUT_PATH"
