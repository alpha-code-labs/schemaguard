package shadowdb

import (
	"strings"
	"testing"
)

func TestDockerUnavailableMessageContainsActionablePhrases(t *testing.T) {
	msg := DockerUnavailableMessage

	// The message is committed — these substrings must remain stable
	// so the CLI can rely on them and tests catch accidental edits.
	required := []string{
		"Docker is required but unavailable.",
		"SchemaGuard v1 is Docker-only",
		"deferred to\nv1.5",
		"Docker is installed",
		"Docker daemon is running",
		"docker version",
		"schemaguard check",
	}
	for _, need := range required {
		if !strings.Contains(msg, need) {
			t.Errorf("DockerUnavailableMessage missing required phrase %q\n\nfull message:\n%s", need, msg)
		}
	}
}

func TestParseDockerPortAcceptsTypicalOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{
			name: "ipv4 only",
			in:   "0.0.0.0:55842\n",
			want: 55842,
		},
		{
			name: "ipv4 and ipv6",
			in:   "0.0.0.0:55842\n[::]:55842\n",
			want: 55842,
		},
		{
			name: "leading whitespace",
			in:   "   0.0.0.0:12345  \n",
			want: 12345,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDockerPort(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseDockerPortRejectsEmptyAndGarbage(t *testing.T) {
	cases := []string{
		"",
		"\n\n",
		"not a port",
		":notanumber",
	}
	for _, in := range cases {
		if _, err := parseDockerPort(in); err == nil {
			t.Errorf("parseDockerPort(%q) expected error, got nil", in)
		}
	}
}
