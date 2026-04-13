package report

import (
	"fmt"
	"strings"
	"time"
)

// DefaultMarkdownBudget is the committed maximum number of
// characters the Markdown formatter may emit before truncation
// kicks in. It is conservative relative to GitHub's actual PR
// comment size limit so the output always fits with headroom
// for the Action's own wrapping.
//
// Recorded in docs/DECISIONS.md; pinned by
// TestDefaultMarkdownBudgetMatchesDecision.
const DefaultMarkdownBudget = 55000

// TruncationFooter is the exact notice appended to the Markdown
// report when one or more findings had to be dropped to fit the
// size budget. Consumers can grep for this marker to detect
// truncation without parsing the Markdown.
const TruncationFooter = "⚠️ **Report truncated** — %d additional lower-severity finding(s) omitted to fit the PR comment size budget."

// FormatMarkdown renders a Report as a PR-comment-sized Markdown
// document. It iterates the canonical group order, emits a level-3
// heading and a compact table per non-empty group, and appends a
// small italic footer line. If the rendered output would exceed
// DefaultMarkdownBudget characters, lowest-severity findings are
// progressively dropped and a truncation notice is appended.
func FormatMarkdown(r *Report) string {
	return formatMarkdownWithBudget(r, DefaultMarkdownBudget)
}

// formatMarkdownWithBudget is the core renderer. The budget
// parameter is exposed so tests can force truncation with small
// inputs — FormatMarkdown is the production caller and always
// passes DefaultMarkdownBudget.
func formatMarkdownWithBudget(r *Report, budget int) string {
	out := renderMarkdown(r, 0)
	if len(out) <= budget {
		return out
	}

	// Progressive drop. Work on a copy so the caller's Report is
	// not mutated. We drop from the tail of the findings slice
	// (the lowest-severity, latest-group, latest-object) until the
	// rendered output fits.
	r2 := *r
	r2.Findings = append([]Finding(nil), r.Findings...)

	for len(r2.Findings) > 0 {
		r2.Findings = r2.Findings[:len(r2.Findings)-1]
		dropped := len(r.Findings) - len(r2.Findings)
		candidate := renderMarkdown(&r2, dropped)
		if len(candidate) <= budget {
			return candidate
		}
	}

	// Even the header, summary, and footer alone blow the budget.
	// Return the smallest possible stub with the truncation
	// notice. This is a degenerate case that should not happen in
	// practice with the committed default budget.
	return renderMarkdownHeaderOnly(r, len(r.Findings))
}

// renderMarkdown produces the Markdown output. If droppedFindings
// is > 0, a truncation notice is appended at the end.
func renderMarkdown(r *Report, droppedFindings int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## %s %s — %s\n\n",
		r.Verdict.Emoji(),
		r.Verdict.Label(),
		r.Summary,
	)

	for _, g := range CanonicalGroupOrder {
		findings := r.FindingsByGroup(g)
		if len(findings) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", g.Title())
		writeMarkdownTable(&b, findings)
		fmt.Fprintln(&b)
	}

	if len(r.Findings) == 0 && droppedFindings == 0 {
		fmt.Fprintln(&b, "No findings. Migration is safe to merge.")
		fmt.Fprintln(&b)
	}

	if droppedFindings > 0 {
		fmt.Fprintf(&b, TruncationFooter+"\n\n", droppedFindings)
	}

	fmt.Fprintln(&b, markdownFooter(r.Footer))
	return b.String()
}

// renderMarkdownHeaderOnly returns the minimum-viable Markdown
// output when even a pre-trimmed report exceeds the budget. Used
// only as a fallback — the default budget is large enough that
// realistic runs never hit this path.
func renderMarkdownHeaderOnly(r *Report, dropped int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s %s — %s\n\n",
		r.Verdict.Emoji(),
		r.Verdict.Label(),
		r.Summary,
	)
	fmt.Fprintf(&b, TruncationFooter+"\n\n", dropped)
	fmt.Fprintln(&b, markdownFooter(r.Footer))
	return b.String()
}

func writeMarkdownTable(b *strings.Builder, findings []Finding) {
	fmt.Fprintln(b, "| Severity | Object | Impact | Reason |")
	fmt.Fprintln(b, "|---|---|---|---|")
	for _, f := range findings {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n",
			markdownEscape(string(f.Severity)),
			markdownEscape(f.Object),
			markdownEscape(f.Impact),
			markdownEscape(f.Reason),
		)
	}
}

// markdownEscape protects table cells from pipe characters and
// newlines that would break the table layout. It is intentionally
// minimal — the source strings come from analyzers that do not
// produce Markdown formatting themselves.
func markdownEscape(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func markdownFooter(f Footer) string {
	parts := []string{}
	if f.ToolVersion != "" {
		parts = append(parts, "SchemaGuard `"+f.ToolVersion+"`")
	}
	if f.RunDuration > 0 {
		parts = append(parts, "run "+f.RunDuration.Round(10*time.Millisecond).String())
	}
	if f.MigrationDuration > 0 {
		parts = append(parts, "migration "+f.MigrationDuration.Round(time.Millisecond).String())
	}
	if f.RestoreDuration > 0 {
		parts = append(parts, "restore "+f.RestoreDuration.Round(time.Millisecond).String())
	}
	if f.ShadowDBImage != "" {
		img := "shadow `" + f.ShadowDBImage + "`"
		if f.ShadowDBSizeBytes > 0 {
			img += " (" + humanBytes(f.ShadowDBSizeBytes) + ")"
		}
		parts = append(parts, img)
	}
	line := ""
	if len(parts) > 0 {
		line = "_" + strings.Join(parts, " · ") + "_"
	}
	if f.DocsURL != "" {
		if line != "" {
			line += "\n\n"
		}
		line += "[Docs](" + f.DocsURL + ")"
	}
	return line
}
