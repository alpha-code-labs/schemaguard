package report

import (
	"bytes"
	"encoding/json"
)

// FormatJSON renders a Report as a pretty-printed JSON document
// suitable for scripts and CI consumers.
//
// The output always includes the schemaVersion field and uses
// stable, lowerCamelCase key names. Durations (run, restore,
// migration) are serialized as millisecond integers via a small
// adapter so consumers do not have to parse Go's time.Duration
// string format.
//
// Any incompatible schema change (field rename, removal, type
// change) must bump SchemaVersion — see build.go.
func FormatJSON(r *Report) ([]byte, error) {
	adapted := toJSON(r)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(adapted); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// jsonReport is the on-the-wire shape of a Report. It exists so
// durations are serialized as millisecond integers rather than the
// default Go nanosecond integers that leak out of time.Duration's
// default MarshalJSON.
type jsonReport struct {
	SchemaVersion string     `json:"schemaVersion"`
	Verdict       Verdict    `json:"verdict"`
	Summary       string     `json:"summary"`
	Findings      []Finding  `json:"findings"`
	Footer        jsonFooter `json:"footer"`
}

type jsonFooter struct {
	ToolVersion         string `json:"toolVersion,omitempty"`
	RunDurationMs       int64  `json:"runDurationMs,omitempty"`
	RestoreDurationMs   int64  `json:"restoreDurationMs,omitempty"`
	MigrationDurationMs int64  `json:"migrationDurationMs,omitempty"`
	ShadowDBImage       string `json:"shadowDbImage,omitempty"`
	ShadowDBSizeBytes   int64  `json:"shadowDbSizeBytes,omitempty"`
	DocsURL             string `json:"docsUrl,omitempty"`
}

func toJSON(r *Report) jsonReport {
	// Ensure findings is a non-nil slice so JSON encodes `[]` not
	// `null` — consumers prefer a stable empty-array shape.
	findings := r.Findings
	if findings == nil {
		findings = []Finding{}
	}
	return jsonReport{
		SchemaVersion: r.SchemaVersion,
		Verdict:       r.Verdict,
		Summary:       r.Summary,
		Findings:      findings,
		Footer: jsonFooter{
			ToolVersion:         r.Footer.ToolVersion,
			RunDurationMs:       r.Footer.RunDuration.Milliseconds(),
			RestoreDurationMs:   r.Footer.RestoreDuration.Milliseconds(),
			MigrationDurationMs: r.Footer.MigrationDuration.Milliseconds(),
			ShadowDBImage:       r.Footer.ShadowDBImage,
			ShadowDBSizeBytes:   r.Footer.ShadowDBSizeBytes,
			DocsURL:             r.Footer.DocsURL,
		},
	}
}
