package cli

// Exit codes defined by docs/build_spec.md and docs/DECISIONS.md.
//
// A product finding (including a migration that halts on its own SQL error)
// returns ExitRed. A tool malfunction (bad inputs, snapshot restore failure,
// Docker unavailable, internal crash) returns ExitToolError. CI systems must
// be able to distinguish "the tool says stop" from "the tool broke", so the
// two must never collapse onto the same code.
const (
	// ExitGreen — no significant findings.
	ExitGreen = 0

	// ExitYellow — caution-level findings. Merge with care.
	ExitYellow = 1

	// ExitRed — stop-level findings. Do not merge. Includes migrations that
	// halted on their own SQL error: that is a product finding, not a tool
	// malfunction.
	ExitRed = 2

	// ExitToolError — the tool could not complete its work. Bad inputs,
	// snapshot restore failure, Docker unavailable, or internal crash.
	// Must be clearly distinguishable from ExitRed.
	ExitToolError = 3
)
