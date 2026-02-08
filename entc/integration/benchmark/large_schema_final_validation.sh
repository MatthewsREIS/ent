#!/usr/bin/env bash

set -euo pipefail

export LC_ALL=C

ROOT="$(git rev-parse --show-toplevel)"
RUNS="${RUNS:-3}"
THRESHOLD_PERCENT="${THRESHOLD_PERCENT:-35}"

if ! [[ "$RUNS" =~ ^[0-9]+$ ]] || (( RUNS < 1 )); then
	echo "RUNS must be a positive integer, got: $RUNS" >&2
	exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required for parsing benchmark artifacts" >&2
	exit 1
fi

BASELINE_PATH="${1:-$ROOT/entc/integration/benchmark/artifacts/large-schema-baseline.json}"
if [[ "$BASELINE_PATH" != /* ]]; then
	BASELINE_PATH="$ROOT/$BASELINE_PATH"
fi

SUMMARY_PATH="${2:-$ROOT/entc/integration/benchmark/artifacts/large-schema-final-validation.json}"
if [[ "$SUMMARY_PATH" != /* ]]; then
	SUMMARY_PATH="$ROOT/$SUMMARY_PATH"
fi

CANDIDATE_PATH="${3:-$ROOT/entc/integration/benchmark/artifacts/large-schema-split-final.json}"
if [[ "$CANDIDATE_PATH" != /* ]]; then
	CANDIDATE_PATH="$ROOT/$CANDIDATE_PATH"
fi

if [[ ! -f "$BASELINE_PATH" ]]; then
	echo "baseline artifact not found: $BASELINE_PATH" >&2
	exit 1
fi

SPLIT_FEATURES="${SPLIT_FEATURES:-entql,sql/modifier,sql/lock,sql/upsert,sql/execquery,namedges,bidiedges,sql/globalid,split-packages}"
DEFAULT_SPLIT_CODEGEN_CMD="go run -mod=mod entgo.io/ent/cmd/ent generate --feature $SPLIT_FEATURES --template ./ent/template ./ent/schema"
SPLIT_CODEGEN_CMD="${SPLIT_CODEGEN_CMD:-$DEFAULT_SPLIT_CODEGEN_CMD}"

mkdir -p "$(dirname "$SUMMARY_PATH")"
mkdir -p "$(dirname "$CANDIDATE_PATH")"

CODEGEN_CMD="$SPLIT_CODEGEN_CMD" RUNS="$RUNS" "$ROOT/entc/integration/benchmark/large_schema_baseline.sh" "$CANDIDATE_PATH"

baseline_build_wall="$(jq -r '.commands.build.wall_seconds.median' "$BASELINE_PATH")"
baseline_build_rss="$(jq -r '.commands.build.peak_rss_kb.median' "$BASELINE_PATH")"
baseline_codegen_wall="$(jq -r '.commands.codegen.wall_seconds.median' "$BASELINE_PATH")"
baseline_codegen_rss="$(jq -r '.commands.codegen.peak_rss_kb.median' "$BASELINE_PATH")"

candidate_build_wall="$(jq -r '.commands.build.wall_seconds.median' "$CANDIDATE_PATH")"
candidate_build_rss="$(jq -r '.commands.build.peak_rss_kb.median' "$CANDIDATE_PATH")"
candidate_codegen_wall="$(jq -r '.commands.codegen.wall_seconds.median' "$CANDIDATE_PATH")"
candidate_codegen_rss="$(jq -r '.commands.codegen.peak_rss_kb.median' "$CANDIDATE_PATH")"

improvement_percent() {
	local baseline="$1"
	local candidate="$2"
	awk -v b="$baseline" -v c="$candidate" 'BEGIN { printf "%.2f", ((b - c) / b) * 100 }'
}

build_wall_improvement="$(improvement_percent "$baseline_build_wall" "$candidate_build_wall")"
build_rss_improvement="$(improvement_percent "$baseline_build_rss" "$candidate_build_rss")"
codegen_wall_improvement="$(improvement_percent "$baseline_codegen_wall" "$candidate_codegen_wall")"
codegen_rss_improvement="$(improvement_percent "$baseline_codegen_rss" "$candidate_codegen_rss")"

meets_threshold() {
	local improvement="$1"
	local threshold="$2"
	awk -v i="$improvement" -v t="$threshold" 'BEGIN { if (i >= t) print "true"; else print "false" }'
}

pass_build_wall="$(meets_threshold "$build_wall_improvement" "$THRESHOLD_PERCENT")"
pass_build_rss="$(meets_threshold "$build_rss_improvement" "$THRESHOLD_PERCENT")"

overall_pass="false"
if [[ "$pass_build_wall" == "true" && "$pass_build_rss" == "true" ]]; then
	overall_pass="true"
fi

timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cat >"$SUMMARY_PATH" <<EOF
{
  "name": "entc-large-schema-final-validation",
  "timestamp_utc": "$timestamp",
  "baseline_artifact": "${BASELINE_PATH#$ROOT/}",
  "candidate_artifact": "${CANDIDATE_PATH#$ROOT/}",
  "runs": $RUNS,
  "threshold_percent": $THRESHOLD_PERCENT,
  "metrics": {
    "baseline": {
      "codegen_wall_seconds_median": $baseline_codegen_wall,
      "codegen_peak_rss_kb_median": $baseline_codegen_rss,
      "build_wall_seconds_median": $baseline_build_wall,
      "build_peak_rss_kb_median": $baseline_build_rss
    },
    "candidate": {
      "codegen_wall_seconds_median": $candidate_codegen_wall,
      "codegen_peak_rss_kb_median": $candidate_codegen_rss,
      "build_wall_seconds_median": $candidate_build_wall,
      "build_peak_rss_kb_median": $candidate_build_rss
    },
    "improvement_percent": {
      "codegen_wall": $codegen_wall_improvement,
      "codegen_peak_rss": $codegen_rss_improvement,
      "build_wall": $build_wall_improvement,
      "build_peak_rss": $build_rss_improvement
    }
  },
  "threshold_checks": {
    "build_wall_gte_threshold": $pass_build_wall,
    "build_peak_rss_gte_threshold": $pass_build_rss
  },
  "pass": $overall_pass
}
EOF

echo "Wrote final validation summary to $SUMMARY_PATH"
if [[ "$overall_pass" != "true" ]]; then
	echo "performance thresholds were not met (required >= ${THRESHOLD_PERCENT}% for build wall and build peak RSS)" >&2
	exit 1
fi
