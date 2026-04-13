package cli

import "testing"

func TestExitCodeValuesAreStable(t *testing.T) {
	if ExitGreen != 0 {
		t.Errorf("ExitGreen must be 0, got %d", ExitGreen)
	}
	if ExitYellow != 1 {
		t.Errorf("ExitYellow must be 1, got %d", ExitYellow)
	}
	if ExitRed != 2 {
		t.Errorf("ExitRed must be 2, got %d", ExitRed)
	}
	if ExitToolError != 3 {
		t.Errorf("ExitToolError must be 3, got %d", ExitToolError)
	}
}

func TestExitCodesAreDistinct(t *testing.T) {
	codes := []int{ExitGreen, ExitYellow, ExitRed, ExitToolError}
	seen := make(map[int]bool, len(codes))
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate exit code: %d", c)
		}
		seen[c] = true
	}
}
