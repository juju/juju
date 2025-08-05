// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"context"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// jobCheckMaxInterval is the maximum time between checks of the removal table,
// to see if any jobs need processing.
const jobCheckMaxInterval = 10 * time.Second

// Config holds configuration required to run the removal worker.
type Config struct {

	// RemovalService supplies the removal domain logic to the worker.
	RemovalService RemovalService

	// Clock is used by the worker to create timers.
	Clock Clock

	// Logger logs stuff.
	Logger logger.Logger
}

// Validate ensures that the configuration is
// correctly populated for worker operation.
func (config Config) Validate() error {
	if config.RemovalService == nil {
		return errors.New("nil RemovalService not valid").Add(coreerrors.NotValid)
	}
	if config.Clock == nil {
		return errors.New("nil Clock not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

type removalWorker struct {
	catacomb catacomb.Catacomb

	cfg    Config
	runner *worker.Runner
}

// NewWorker starts a new removal worker based
// on the input configuration and returns it.
func NewWorker(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "removal",
		IsFatal:       func(error) bool { return false },
		ShouldRestart: func(error) bool { return false },
		Logger:        internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &removalWorker{
		cfg: cfg,
		// Scheduled removal jobs never restart and never
		// propagate their errors up to the worker.
		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "removal",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

func (w *removalWorker) loop() (err error) {
	ctx := w.catacomb.Context(context.Background())

	watch, err := w.cfg.RemovalService.WatchRemovals(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(watch); err != nil {
		return errors.Capture(err)
	}

	timer := w.cfg.Clock.NewTimer(jobCheckMaxInterval)
	defer timer.Stop()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case jobIDs := <-watch.Changes():
			w.cfg.Logger.Infof(ctx, "got removal job changes: %v", jobIDs)
			if err := w.processRemovalJobs(ctx); err != nil {
				return errors.Capture(err)
			}
			timer.Reset(jobCheckMaxInterval)
		case <-timer.Chan():
			if err := w.processRemovalJobs(ctx); err != nil {
				return errors.Capture(err)
			}
			timer.Reset(jobCheckMaxInterval)
		}
	}
}

// processRemovalJobs interrogates *all* current jobs scheduled for removal.
// For each one whose scheduled start time has passed, we check to see if there
// is an entry in our runner for it. If there is, it is already being processed
// and we ignore it. Otherwise, it is commenced in a new runner.
// This is safe due to the following conditions:
// - This is the only method adding workers to the runner.
// - It is only invoked from cases in the main event loop, so is Goroutine safe.
func (w *removalWorker) processRemovalJobs(ctx context.Context) error {
	jobs, err := w.cfg.RemovalService.GetAllJobs(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	running := set.NewStrings(w.runner.WorkerNames()...)
	log := w.cfg.Logger

	for _, j := range jobs {
		id := j.UUID.String()

		// The worker for this job may have completed since we retrieved the
		// worker names, but we don't fuss over it. The job will be picked up
		// again in at most [jobCheckMaxInterval].
		if running.Contains(id) {
			log.Debugf(ctx, "removal job %q already running", id)
			continue
		}

		if j.ScheduledFor.After(w.cfg.Clock.Now().UTC()) {
			log.Debugf(ctx, "removal job %q not due until %s", id, j.ScheduledFor.Format(time.RFC3339))
			continue
		}

		w.cfg.Logger.Infof(ctx, "scheduling job %q", id)
		if err := w.runner.StartWorker(ctx, id, newJobWorker(w.cfg.RemovalService, j)); err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

// Report returns data for display in the dependency engine report.
// In this case, it simply reports on all jobs in the runner.
func (w *removalWorker) Report() map[string]any {
	return w.runner.Report()
}

// Kill (worker.Worker) tells the worker to stop and return from its loop.
func (w *removalWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait (worker.Worker) waits for the worker to stop,
// and returns the error with which it exited.
func (w *removalWorker) Wait() error {
	return w.catacomb.Wait()
}

// jobWorker exists to expose enough of a tomb to satisfy [worker.Worker],
// and to generate a report.
type jobWorker struct {
	tomb tomb.Tomb
	job  removal.Job
}

// newJobWorker returns a closure suitable for passing to
// a runner's StartWorker method.
// It uses the input service to run the input removal job.
func newJobWorker(svc RemovalService, job removal.Job) func(context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		w := jobWorker{job: job}
		w.tomb.Go(func() error {
			return svc.ExecuteJob(w.tomb.Context(context.Background()), job)
		})
		return &w, nil
	}
}

// Report returns information about the removal job that the worker is running.
func (w *jobWorker) Report() map[string]any {
	return map[string]any{
		"job-type":       w.job.RemovalType,
		"removal-entity": w.job.EntityUUID,
		"force":          w.job.Force,
	}
}

// Kill (worker.Worker) tells the worker to stop running the job and return.
func (w *jobWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the worker to stop,
// and returns the error with which it exited.
func (w *jobWorker) Wait() error {
	return w.tomb.Wait()
}
