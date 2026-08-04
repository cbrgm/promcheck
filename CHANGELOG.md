# Changelog

All notable changes to this project are documented in this file.

## v2.0.0

### Added

* Support for Prometheus 3.x (bumped `prometheus/prometheus` to v0.313.2).
* UTF-8 metric and label name support.
* Server-side rule filtering via `--check.match`, so Prometheus only returns rules matching the given PromQL label matchers instead of `promcheck` filtering them client-side.
* Structured logging via `log/slog` (`--log.json`, `--log.level`).
* `--output.only-failing` restricts output (any format) to rules that have at least one selector without a result. Summary totals still reflect the full run.
* Colored output now honors `NO_COLOR` (see [no-color.org](https://no-color.org/)) and auto-disables when stdout isn't a terminal (e.g. piped or redirected output), on top of `--output.no-color`.
* When checking rule files, `promcheck` now honors a rule group's `query_offset`, probing that group's selectors against data from `now - query_offset` instead of always `now`. This cuts down on false "no result" findings for groups that intentionally evaluate against delayed data. Live-instance mode still probes at `now`, since the Prometheus rules API doesn't expose `query_offset`.
* New exporter metrics: `promcheck_build_info` (version/revision/goversion), `promcheck_last_run_timestamp_seconds`, `promcheck_run_duration_seconds`, and `promcheck_run_errors_total`.
* A documented, stable exit-code contract (`0`/`1`/`2`/`3`), see Breaking changes below.

### Changed

* Selector probes are now bounded by `--check.concurrency` (default `8`) instead of a fixed per-request delay.
* Report output (tree, json, yaml) is now sorted deterministically by file, group, and rule name, instead of following the non-deterministic order concurrent probes finish in.
* A failed exporter check cycle no longer tears down the exporter: it's logged, `promcheck_run_errors_total` is incremented, and the exporter keeps running on its next interval.
* Internal packages moved under `internal/{checker,report,metrics}`. This doesn't affect the module path or `make build`/`go build ./...`.

### Fixed

* Selectors following an ignored `ALERTS` matcher are now probed correctly (they used to be silently skipped).
* Non-vector query results no longer cause a panic.
* Empty rule sets now exit with a non-zero code instead of silently succeeding.
* Probe errors now propagate instead of being swallowed.
* The failed/total ratio no longer produces `NaN` when there are no selectors to probe.

### Breaking changes

* `--check.delay` has been removed in favor of `--check.concurrency`. There is no direct equivalent: `--check.delay` throttled requests with a fixed delay, `--check.concurrency` bounds how many probes run in parallel.
* Running `promcheck` against an empty rule set now exits with a non-zero code. Previously it exited `0`.
* The `csv` output format has been removed. Use `json` or `yaml` instead.
* `--metrics.profile` (pprof profiling) and `--metrics.runtime` (Go runtime metrics) now default to `false`. Both used to be on by default; enable them explicitly if you rely on that data.
* json/yaml output always includes `groups_total`, `rules_total`, `selectors_failed_total`, `selectors_success_total`, `ratio_failed_total`, and `results` (an empty array `[]` when there's nothing to report), even when a value is zero. Previously zero-valued fields were omitted. Scripts that checked for a field's absence should check its value instead.
* Exit codes are now a documented, stable contract instead of always exiting `1` on any failure:
  * `0` - completed, no findings (or a non-strict run)
  * `1` - `--strict` was set and one or more selectors had no results
  * `2` - configuration error (invalid `--check.ignore-selector`/`--check.ignore-group` regexp, or nothing to check)
  * `3` - runtime failure while probing (connection, query, or parse error)

### Migrating from v1

* `--check.delay N` -> `--check.concurrency N`. There's no exact numeric equivalent; pick a concurrency value that matches how much load your Prometheus instance can take.
* If you relied on `promcheck` exiting `0` for empty rule sets (e.g. in CI when no rule files matched), expect a non-zero exit now and adjust your pipeline accordingly.
* `--output.format=csv` no longer works. Switch to `--output.format=json` or `--output.format=yaml`.
