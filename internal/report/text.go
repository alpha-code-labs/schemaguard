package report

import (
	"fmt"
	"strings"
	"time"
)

// FormatText renders a Report as a scannable plain-text string for
// stdout. Empty groups are omitted. The output is intentionally
// narrow (< 80 columns for the body) so it reads cleanly in a
// terminal window.
//
// The format is:
//
//	🔴 STOP — <summary>
//
//	Migration Execution
//	  [stop] object — impact
//	         reason
//
//	Lock Risk
//	  [stop] object — impact
//	         reason
//
//	Query Plan Regressions
//	  [caution] object — impact
//	            reason
//
//	───
//	SchemaGuard 0.0.0-dev · run 1.2s · migration 0.5s · restore 0.3s · shadow postgres:16-alpine (12 MB)
//	https://github.com/alpha-code-labs/schemaguard
func FormatText(r *Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s — %s\n\n",
		r.Verdict.Emoji(),
		strings.ToUpper(r.Verdict.Label()),
		r.Summary,
	)

	for _, g := range CanonicalGroupOrder {
		findings := r.FindingsByGroup(g)
		if len(findings) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s\n", g.Title())
		for _, f := range findings {
			writeTextFinding(&b, f)
		}
		fmt.Fprintln(&b)
	}

	if len(r.Findings) == 0 {
		fmt.Fprintln(&b, "No findings. Migration is safe to merge.")
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "───")
	fmt.Fprintln(&b, textFooter(r.Footer))
	return b.String()
}

func writeTextFinding(b *strings.Builder, f Finding) {
	header := fmt.Sprintf("  [%s] %s", f.Severity, f.Object)
	if f.Impact != "" {
		header += " — " + f.Impact
	}
	fmt.Fprintln(b, header)
	if f.Reason != "" {
		fmt.Fprintf(b, "        %s\n", f.Reason)
	}
}

func textFooter(f Footer) string {
	parts := []string{}
	if f.ToolVersion != "" {
		parts = append(parts, "SchemaGuard "+f.ToolVersion)
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
		img := "shadow " + f.ShadowDBImage
		if f.ShadowDBSizeBytes > 0 {
			img += " (" + humanBytes(f.ShadowDBSizeBytes) + ")"
		}
		parts = append(parts, img)
	}
	line := strings.Join(parts, " · ")
	if f.DocsURL != "" {
		if line != "" {
			line += "\n"
		}
		line += f.DocsURL
	}
	return line
}

// humanBytes renders an integer byte count as a rough human-
// readable size. It is purely cosmetic for the footer and is not
// used in the JSON layout.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}
