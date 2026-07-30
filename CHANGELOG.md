# Changelog

All notable changes to this project are documented in this file.

## v2.0.0

### Added

* Support for Prometheus 3.x (bumped `prometheus/prometheus` to v0.313.2).
* UTF-8 metric and label name support.
* Server-side rule filtering via `--check.match`, so Prometheus only returns rules matching the given PromQL label matchers instead of `promcheck` filtering them client-side.
* Structured logging via `log/slog` (`--log.json`, `--log.level`).

### Changed

* Selector probes are now bounded by `--check.concurrency` (default `8`) instead of a fixed per-request delay.

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

### Migrating from v1

* `--check.delay N` -> `--check.concurrency N`. There's no exact numeric equivalent; pick a concurrency value that matches how much load your Prometheus instance can take.
* If you relied on `promcheck` exiting `0` for empty rule sets (e.g. in CI when no rule files matched), expect a non-zero exit now and adjust your pipeline accordingly.
* `--output.format=csv` no longer works. Switch to `--output.format=json` or `--output.format=yaml`.
