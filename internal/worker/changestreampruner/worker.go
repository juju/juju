// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"
	"sort"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
)

const (
	// defaultPruneMinInterval is the default minimum interval at which the
	// pruner will run.
	defaultPruneMinInterval = time.Second * 5
	// defaultPruneMaxInterval is the default maximum interval at which the
	// pruner will run.
	defaultPruneMaxInterval = time.Second * 30

	// defaultWindowDuration is the default duration of the window in which
	// the pruner will select the lower bound of the watermark. If any
	// watermarks are outside of this window, they will not be selected and the
	// pruner will discard those watermarks.
	defaultWindowDuration = time.Minute * 10
)

var (
	// backOffStrategy is the default backoff strategy used by the pruner.
	backOffStrategy = retry.ExpBackoff(defaultPruneMinInterval, defaultPruneMaxInterval, 1.5, false)
)

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter = coredatabase.DBGetter

// WorkerConfig encapsulates the configuration options for the
// changestream worker.
type WorkerConfig struct {
	DBGetter DBGetter
	Clock    clock.Clock
	Logger   logger.Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.DBGetter == nil {
		return errors.NotValidf("missing DBGetter")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

// Pruner defines a worker that will truncate the change log.
type Pruner struct {
	tomb tomb.Tomb

	cfg WorkerConfig

	// windows holds the last window for each namespace. This is used to
	// determine if the change stream is keeping up with the pruner. If the
	// watermark is outside of the window, we should log a warning message.
	windows map[string]window
}

// New creates a new Pruner.
func newWorker(cfg WorkerConfig) (*Pruner, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	pruner := &Pruner{
		cfg:     cfg,
		windows: make(map[string]window),
	}

	pruner.tomb.Go(pruner.loop)

	return pruner, nil
}

// Kill is part of the worker.Worker interface.
func (w *Pruner) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Pruner) Wait() error {
	return w.tomb.Wait()
}

func (w *Pruner) loop() error {
	timer := w.cfg.Clock.NewTimer(defaultPruneMinInterval)
	defer timer.Stop()

	var attempts int
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case <-timer.Chan():
			// Attempt to prune, if there is any critical error, kill the
			// worker, which should force a restart.
			pruned, err := w.prune()
			if err != nil {
				return errors.Trace(err)
			}

			// If nothing was pruned, increment the attempts counter, otherwise
			// reset it. This should wind out the backoff strategy if there is
			// nothing to prune, thus reducing the frequency of the pruner.
			if len(pruned) == 0 {
				attempts++
			} else {
				attempts = 0
			}

			timer.Reset(backOffStrategy(0, attempts))
		}
	}
}

func (w *Pruner) prune() (map[string]int64, error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	traceEnabled := w.cfg.Logger.IsLevelEnabled(logger.TRACE)
	if traceEnabled {
		w.cfg.Logger.Tracef(ctx, "Starting pruning change log")
	}

	db, err := w.cfg.DBGetter.GetDB(coredatabase.ControllerNS)
	if err != nil {
		return nil, errors.Trace(err)
	}

	query, err := sqlair.Prepare(`
SELECT namespace AS &ModelNamespace.namespace, model_uuid AS &ModelNamespace.uuid
FROM model_namespace;
	`, ModelNamespace{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var modelNamespaces []ModelNamespace
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, query).GetAll(&modelNamespaces))
	})
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
	}

	// To ensure we always prune the change log for the controller, we add it
	// to the list of models to prune.
	modelNamespaces = append([]ModelNamespace{
		{Namespace: coredatabase.ControllerNS},
	}, modelNamespaces...)

	// Prune each and every model found in the model list.
	pruned := make(map[string]int64)
	modelNames := make(map[string]struct{})
	for _, mn := range modelNamespaces {
		// Store the model name in a map. We can't use pruned map for the
		// tracking of namespaces, because if there is an error we might
		// accidentally remove a window for a model that hasn't been deleted.
		modelNames[mn.Namespace] = struct{}{}

		p, err := w.pruneModel(ctx, mn.Namespace)
		if err != nil {
			// If the database is dead, continue on to the next model, as we
			// don't want to kill the worker.
			if errors.Is(err, coredatabase.ErrDBDead) {
				continue
			}
			// If there is an error, continue on to the next model, as we don't
			// want to kill the worker.
			w.cfg.Logger.Infof(ctx, "Error pruning model %q: %v", mn.UUID, err)
			continue
		}

		if traceEnabled {
			w.cfg.Logger.Tracef(ctx, "Pruned %d change logs for model %q", pruned, mn.UUID)
		}

		pruned[mn.Namespace] = p
	}

	// Ensure we clean up the windows for any models that have been deleted.
	// The absence of a model in the modelNames list indicates that the model
	// has been deleted and we should remove the window.
	for namespace := range w.windows {
		if _, ok := modelNames[namespace]; !ok {
			delete(w.windows, namespace)
		}
	}

	if traceEnabled {
		w.cfg.Logger.Tracef(ctx, "Finished pruning change log")
	}

	return pruned, nil
}

func (w *Pruner) pruneModel(ctx context.Context, namespace string) (int64, error) {
	db, err := w.cfg.DBGetter.GetDB(namespace)
	if err != nil {
		return -1, errors.Trace(err)
	}

	var pruned int64
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Locate the lowest watermark, this is the watermark that we will
		// use to prune the change log.
		lowest, err := w.locateLowestWatermark(ctx, tx, namespace)
		if err != nil {
			return errors.Annotatef(err, "failed to locate lowest watermark")
		}

		// Prune the change log, using the lowest watermark.
		pruned, err = w.deleteChangeLog(ctx, tx, lowest)
		return errors.Annotatef(err, "failed to prune change log")
	})
	return pruned, errors.Trace(err)
}

var selectWitnessQuery = sqlair.MustPrepare(`SELECT (controller_id, lower_bound, updated_at) AS (&Watermark.*) FROM change_log_witness;`, Watermark{})

func (w *Pruner) locateLowestWatermark(ctx context.Context, tx *sqlair.TX, namespace string) (Watermark, error) {
	// Gather all the valid watermarks, post row pruning. These include
	// the controller id which we know are valid based on the
	// controller_node table. If at any point we delete rows from the
	// change_log_witness table, the change stream will put the witness
	// back in place after the next change log is written.
	var watermarks []Watermark
	if err := tx.Query(ctx, selectWitnessQuery).GetAll(&watermarks); errors.Is(err, sqlair.ErrNoRows) {
		// Nothing to do if there are no watermarks.
		return Watermark{}, nil
	} else if err != nil {
		return Watermark{}, errors.Trace(err)
	}

	// Gather all the watermarks that are within the windowed time period.
	// If there are no watermarks within the window, then we can assume
	// that the stream is keeping up and we don't need to prune anything.
	sorted := sortWatermarks(namespace, watermarks)

	// Find the first and last watermark in the sorted list, this is our
	// window. It should hold the start and the end of the window.
	watermarkView := window{
		start: sorted[0].UpdatedAt,
		end:   sorted[len(sorted)-1].UpdatedAt,
	}

	// If the watermark is outside of the window, we should log a warning
	// message to indicate that the change stream is not keeping up. Only if
	// the watermark is different from the last window, as we don't want to
	// spam the logs if there are no changes.
	now := w.cfg.Clock.Now()
	timeView := window{
		start: now.Add(-defaultWindowDuration),
		end:   now,
	}
	if !timeView.Contains(watermarkView) && !watermarkView.Equals(w.windows[namespace]) {
		w.cfg.Logger.Warningf(ctx, "namespace %s watermarks %q are outside of window, check logs to see if the change stream is keeping up", namespace, sorted[0].ControllerID)
	}

	// Save the last window for the next iteration.
	w.windows[namespace] = watermarkView

	return sorted[0], nil
}

var deleteQuery = sqlair.MustPrepare(`DELETE FROM change_log WHERE id <= $M.id;`, sqlair.M{})

func (w *Pruner) deleteChangeLog(ctx context.Context, tx *sqlair.TX, lowest Watermark) (int64, error) {
	// Delete all the change logs that are lower than the lowest watermark.
	var outcome sqlair.Outcome
	if err := tx.Query(ctx, deleteQuery, sqlair.M{"id": lowest.LowerBound}).Get(&outcome); err != nil {
		return -1, errors.Trace(err)
	}
	pruned, err := outcome.Result().RowsAffected()
	return pruned, errors.Trace(err)
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *Pruner) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func sortWatermarks(namespace string, watermarks []Watermark) []Watermark {
	// If there is only one watermark, just use that one and return out early.
	if len(watermarks) == 1 {
		return watermarks
	}

	// Sort the watermarks by the lower bound.
	sort.Slice(watermarks, func(i, j int) bool {
		if watermarks[i].LowerBound == watermarks[j].LowerBound {
			return watermarks[i].UpdatedAt.Before(watermarks[j].UpdatedAt)
		}
		return watermarks[i].LowerBound < watermarks[j].LowerBound
	})

	return watermarks
}

// ModelNamespace represents a model and the associated DQlite namespace that it
// uses.
type ModelNamespace struct {
	UUID      string `db:"uuid"`
	Namespace string `db:"namespace"`
}

// Watermark represents a row from the change_log_witness table.
type Watermark struct {
	ControllerID string    `db:"controller_id"`
	LowerBound   int64     `db:"lower_bound"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type window struct {
	start, end time.Time
}

// Contains returns true if the window contains the given time.
func (w window) Contains(o window) bool {
	if w.Equals(o) {
		return true
	}
	return w.start.Before(o.start) && w.end.After(o.end)
}

// Equals returns true if the window is equal to the given window.
func (w window) Equals(o window) bool {
	return w.start.Equal(o.start) && w.end.Equal(o.end)
}

func (w window) String() string {
	return w.start.Format(time.RFC3339) + " -> " + w.end.Format(time.RFC3339)
}
