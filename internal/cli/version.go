package cli

// Version is the tool version string. It is overridden at build time via
// -ldflags "-X github.com/alpha-code-labs/schemaguard/internal/cli.Version=vX.Y.Z"
// during release builds. Unbuilt / development binaries report the default
// placeholder.
var Version = "0.0.0-dev"
