package cli

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// CleanupHook is a function invoked during shutdown. Hooks run in LIFO
// order (last registered, first executed), which matches the order in
// which resources are typically acquired.
type CleanupHook func()

var (
	cleanupMu    sync.Mutex
	cleanupHooks []CleanupHook
	cleanupOnce  sync.Once
)

// RegisterCleanup appends a hook to the cleanup registry. Hooks are invoked
// exactly once during shutdown, whether shutdown is triggered by a signal
// (SIGINT/SIGTERM) or by normal termination via the stop function returned
// from installSignalHandler.
//
// Milestone 1 does not register any real hooks yet — this is the scaffold
// that later milestones (shadow DB teardown, temp file removal, etc.) will
// plug into.
func RegisterCleanup(h CleanupHook) {
	cleanupMu.Lock()
	cleanupHooks = append(cleanupHooks, h)
	cleanupMu.Unlock()
}

// runCleanup invokes every registered hook in LIFO order. It is safe to
// call more than once — subsequent calls are no-ops via sync.Once.
func runCleanup() {
	cleanupOnce.Do(func() {
		cleanupMu.Lock()
		hooks := cleanupHooks
		cleanupHooks = nil
		cleanupMu.Unlock()
		for i := len(hooks) - 1; i >= 0; i-- {
			hooks[i]()
		}
	})
}

// installSignalHandler sets up a SIGINT / SIGTERM handler. On signal, the
// handler runs every registered cleanup hook and cancels the returned
// context. The caller must invoke the returned stop function before normal
// termination to guarantee cleanup runs even on the non-signal path and to
// deregister the OS signal handler.
func installSignalHandler() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() {
		doneOnce.Do(func() { close(done) })
	}

	go func() {
		select {
		case <-sigCh:
			runCleanup()
			cancel()
		case <-done:
		}
	}()

	stop := func() {
		signal.Stop(sigCh)
		closeDone()
		runCleanup()
		cancel()
	}
	return ctx, stop
}
