// Package report is the v1 unified reporting layer. It consumes the
// structured outputs of the executor, the lock analyzer, and the
// query-plan regression analyzer, and renders them into a single
// cross-formatter Report struct plus a top-line red/yellow/green
// verdict.
//
// Three formatters are provided in M5: text (scannable stdout),
// JSON (stable, versioned schema for machine consumers), and
// Markdown (PR-comment-sized, truncated gracefully when findings
// overflow GitHub's comment size limit).
//
// The report package is the M5 deliverable. It replaces the
// temporary M2+ fmt.Fprintln plumbing that lived in
// internal/cli/check.go through Milestones 2–4. Any downstream
// consumer (the M6 GitHub Action, external integrations, the CI
// log) is meant to read this package's output, not the raw
// analyzer findings.
package report
