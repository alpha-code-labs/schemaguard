// Package lockanalyzer samples pg_locks concurrently with a migration
// execution, attributes observed locks to the statements that took
// them, and produces structured lock-risk findings.
//
// The package is the M3 deliverable (see docs/tasks.md). It is
// intentionally minimal: a Sampler goroutine collects raw Sample rows
// at a fixed interval, an Analyze function merges those samples with
// the statement-execution windows from internal/executor, and the
// result is a flat []Finding slice that the CLI prints today and the
// M5 report generator will consume later.
//
// This package does NOT produce human-readable output. Formatting the
// findings is the caller's concern. Any future report architecture
// (see internal/report — M5) reads Finding directly, not through
// strings produced here.
package lockanalyzer
