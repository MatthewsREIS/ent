# Large-Schema Baseline Benchmark

This directory contains the baseline harness for the split-packages epic.

The harness measures the current baseline for the largest integration fixture:

- `entc/integration/ent/schema` for code generation
- `entc/integration/ent` for package build

## Committed benchmark command

```bash
RUNS=3 ./entc/integration/benchmark/large_schema_baseline.sh
```

Optional output path override:

```bash
RUNS=3 ./entc/integration/benchmark/large_schema_baseline.sh entc/integration/benchmark/artifacts/large-schema-baseline.json
```

## Baseline artifact

The harness writes JSON with:

- generation wall time (`wall_seconds`)
- build wall time (`wall_seconds`)
- peak RSS (`peak_rss_kb`)
- per-run samples and summary stats (`min`, `median`, `max`)
