package lockanalyzer

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// DefaultSamplingInterval is the committed M3 default for how often the
// sampler polls pg_locks against the shadow Postgres backend while the
// migration executor is running.
//
// The value is recorded in docs/DECISIONS.md (Confirmed decision
// "Lock sampling frequency default"). Any change here must be paired
// with an update to that decision entry.
const DefaultSamplingInterval = 50 * time.Millisecond

// sampleQuery returns, for each relation-level lock currently held or
// waited on by a given backend, the lock mode, grant status, and the
// fully qualified relation name. System-schema locks (pg_catalog,
// information_schema, pg_toast) are filtered out — they are noise for
// migration analysis.
//
// $1 is the target backend PID (the migration connection's PID).
const sampleQuery = `
SELECT l.mode,
       l.granted,
       l.relation::regclass::text AS object
FROM pg_locks l
JOIN pg_class c ON l.relation = c.oid
JOIN pg_namespace n ON c.relnamespace = n.oid
WHERE l.pid = $1
  AND l.locktype = 'relation'
  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
`

// Sample is one observation of a single lock held (or waited on) by
// the target backend at a specific wall-clock time.
type Sample struct {
	At      time.Time
	Mode    string
	Granted bool
	Object  string
}

// Sampler polls pg_locks on a fixed interval against a dedicated pgx
// connection (never the migration connection) and collects samples
// for the lifetime of the migration run.
//
// Sampler is started via Run in its own goroutine, stopped by
// cancelling the context passed to Run, and then drained via Samples.
// Run signals completion by closing the Done channel.
type Sampler struct {
	conn      *pgx.Conn
	targetPID uint32
	interval  time.Duration

	mu      sync.Mutex
	samples []Sample
	errs    []error

	done chan struct{}
}

// NewSampler creates a Sampler bound to conn that will poll for locks
// held by the backend identified by targetPID every interval. If
// interval is zero, DefaultSamplingInterval is used.
//
// conn must be a distinct pgx.Conn from the migration connection —
// sharing it would serialize the sampler behind the migration's
// transaction and defeat the purpose.
func NewSampler(conn *pgx.Conn, targetPID uint32, interval time.Duration) *Sampler {
	if interval <= 0 {
		interval = DefaultSamplingInterval
	}
	return &Sampler{
		conn:      conn,
		targetPID: targetPID,
		interval:  interval,
		done:      make(chan struct{}),
	}
}

// Run drives the sampling loop. It blocks until ctx is cancelled.
// Callers typically invoke Run in a goroutine and cancel ctx when the
// migration executor has returned.
//
// Run takes an immediate first sample before entering the ticker loop
// so extremely short migrations still get at least one observation.
// Sample errors are collected but do not stop the loop — a transient
// network blip should not cause the sampler to drop future samples.
func (s *Sampler) Run(ctx context.Context) {
	defer close(s.done)

	// Immediate first sample so very short migrations do not miss the
	// entire lock lifetime between the zeroth tick and ticker[0].
	s.recordSample(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recordSample(ctx)
		}
	}
}

// Done returns a channel that is closed once Run has returned.
// Callers that cancel the sampling context should block on Done before
// reading Samples so the loop has drained.
func (s *Sampler) Done() <-chan struct{} { return s.done }

// Samples returns a copy of every Sample the sampler has recorded so
// far. It is safe to call before Run returns, but callers that want
// the final, complete slice should wait on Done first.
func (s *Sampler) Samples() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Sample, len(s.samples))
	copy(out, s.samples)
	return out
}

// Errors returns every non-nil error the sampling loop encountered.
// These are informational only — the sampler never stops itself on
// query errors, to avoid losing future samples to a transient blip.
func (s *Sampler) Errors() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]error, len(s.errs))
	copy(out, s.errs)
	return out
}

func (s *Sampler) recordSample(ctx context.Context) {
	at := time.Now()
	rows, err := s.conn.Query(ctx, sampleQuery, s.targetPID)
	if err != nil {
		// A cancelled context is expected during shutdown — do not log
		// it as an error.
		if ctx.Err() == nil {
			s.mu.Lock()
			s.errs = append(s.errs, err)
			s.mu.Unlock()
		}
		return
	}
	defer rows.Close()

	var batch []Sample
	for rows.Next() {
		var mode, object string
		var granted bool
		if err := rows.Scan(&mode, &granted, &object); err != nil {
			s.mu.Lock()
			s.errs = append(s.errs, err)
			s.mu.Unlock()
			return
		}
		batch = append(batch, Sample{
			At:      at,
			Mode:    mode,
			Granted: granted,
			Object:  object,
		})
	}
	if err := rows.Err(); err != nil && ctx.Err() == nil {
		s.mu.Lock()
		s.errs = append(s.errs, err)
		s.mu.Unlock()
	}

	if len(batch) == 0 {
		return
	}
	s.mu.Lock()
	s.samples = append(s.samples, batch...)
	s.mu.Unlock()
}
