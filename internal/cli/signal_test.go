package cli

import (
	"sync"
	"testing"
)

// resetCleanupState resets the package-level cleanup registry so tests
// don't bleed into each other. Tests that touch the registry must call
// this at the top.
func resetCleanupState() {
	cleanupMu.Lock()
	cleanupHooks = nil
	cleanupMu.Unlock()
	cleanupOnce = sync.Once{}
}

func TestRegisterCleanupRunsHooksInLIFOOrder(t *testing.T) {
	resetCleanupState()
	defer resetCleanupState()

	var order []int
	RegisterCleanup(func() { order = append(order, 1) })
	RegisterCleanup(func() { order = append(order, 2) })
	RegisterCleanup(func() { order = append(order, 3) })

	runCleanup()

	if len(order) != 3 {
		t.Fatalf("expected 3 hooks to run, got %d", len(order))
	}
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("expected LIFO order [3,2,1], got %v", order)
	}
}

func TestRunCleanupIsIdempotent(t *testing.T) {
	resetCleanupState()
	defer resetCleanupState()

	calls := 0
	RegisterCleanup(func() { calls++ })

	runCleanup()
	runCleanup()
	runCleanup()

	if calls != 1 {
		t.Errorf("expected hook to run exactly once, ran %d times", calls)
	}
}

func TestInstallSignalHandlerStopIsSafe(t *testing.T) {
	resetCleanupState()
	defer resetCleanupState()

	called := false
	RegisterCleanup(func() { called = true })

	ctx, stop := installSignalHandler()
	stop()

	if !called {
		t.Error("expected cleanup hook to run on stop()")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("expected ctx to be cancelled after stop()")
	}

	// Calling stop() twice must not panic.
	stop()
}
