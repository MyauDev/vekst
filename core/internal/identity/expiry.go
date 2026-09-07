package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// expiryInterval is how often abandoned flows and old sessions are swept. The
// sweep is cheap -- two indexed deletes -- and neither table is on a request
// path, so the interval is about keeping the tables small rather than about
// correctness. Expiry itself is enforced by predicates in the queries, so a
// session is dead the moment it expires whether or not the job has run.
const expiryInterval = time.Hour

// ExpiryArgs sweeps expired auth_flows and old sessions.
//
// It takes no organisation, and that is not an omission: these four tables
// carry no tenant data, which is exactly why they are allowlisted out of
// row-level security. Every *other* worker from change 1.1 onward must take
// its tenant identifier from its own arguments.
type ExpiryArgs struct{}

// Kind satisfies river.JobArgs.
func (ExpiryArgs) Kind() string { return "identity_expiry" }

type expiryWorker struct {
	river.WorkerDefaults[ExpiryArgs]
	svc *Service
}

func (w *expiryWorker) Work(ctx context.Context, _ *river.Job[ExpiryArgs]) error {
	return w.svc.expire(ctx)
}

// Register adds this package's worker and its schedule to a River client. It
// is how core/internal/jobs stays generic: the queries against the identity
// tables live here, behind the same package boundary the narrowness check
// draws, rather than in the jobs package.
func (s *Service) Register(w *river.Workers) []*river.PeriodicJob {
	if s == nil {
		return nil
	}
	river.AddWorker(w, &expiryWorker{svc: s})

	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(expiryInterval),
			func() (river.JobArgs, *river.InsertOpts) { return ExpiryArgs{}, nil },
			// Run once at startup so a long-lived deployment is not the only
			// thing that ever sweeps, and a developer sees it work.
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}
}

// expire deletes pending flows past their expiry, and sessions that expired
// longer ago than the retention window. Retention governs how long a dead
// session stays readable to an operator asking what happened; it never governs
// access, which ended at expires_at.
func (s *Service) expire(ctx context.Context) error {
	cutoff := s.now().Add(-s.cfg.SessionRetention)

	var flows, sessions int64
	err := s.database.InSystemTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		q := gendb.New(tx)

		var err error
		if flows, err = q.DeleteExpiredAuthFlows(ctx); err != nil {
			return err
		}
		sessions, err = q.DeleteExpiredSessions(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
		return err
	})
	if err != nil {
		return fmt.Errorf("identity: expiring flows and sessions: %w", err)
	}

	if flows > 0 || sessions > 0 {
		s.log.Info("identity: swept", "auth_flows", flows, "sessions", sessions)
	}
	return nil
}
