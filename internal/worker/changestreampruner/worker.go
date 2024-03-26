// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"
	"math"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
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
	Logger   Logger
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
}

// New creates a new Pruner.
func newWorker(cfg WorkerConfig) (*Pruner, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	pruner := &Pruner{
		cfg: cfg,
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

type window struct {
	start, end time.Time
}

// contains returns true if the window contains the given time.
// Note: This assumes there isn't any clock drift!
func (w *window) contains(t time.Time) bool {
	return t.Compare(w.start) >= 0 && t.Compare(w.end) <= 0
}

func (w *Pruner) prune() (map[string]int64, error) {
	traceEnabled := w.cfg.Logger.IsTraceEnabled()
	if traceEnabled {
		w.cfg.Logger.Tracef("Starting pruning change log")
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	db, err := w.cfg.DBGetter.GetDB(coredatabase.ControllerNS)
	if err != nil {
		return nil, errors.Trace(err)
	}

	query, err := sqlair.Prepare(`SELECT uuid AS &Model.uuid FROM model;`, Model{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var models []Model
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, query).GetAll(&models))
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// To ensure we always prune the change log for the controller, we add it
	// to the list of models to prune.
	models = append([]Model{
		{UUID: coredatabase.ControllerNS},
	}, models...)

	// Prune each and every model found in the model list. This
	pruned := make(map[string]int64)
	for _, model := range models {
		p, err := w.pruneModel(ctx, model.UUID)
		if err != nil {
			// If there is an error, continue on to the next model, as we don't
			// want to kill the worker.
			w.cfg.Logger.Infof("Error pruning model %q: %v", model.UUID, err)
			continue
		}

		if traceEnabled {
			w.cfg.Logger.Tracef("Pruned %d change logs for model %q", pruned, model.UUID)
		}

		pruned[model.UUID] = p
	}

	if traceEnabled {
		w.cfg.Logger.Tracef("Finished pruning change log")
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

// ChangeLog represents a row from the change_log table.
type ChangeLog struct {
	ID int `db:"id"`
}

var (
	selectWitnessQuery = sqlair.MustPrepare(`SELECT (controller_id, lower_bound, updated_at) AS (&Watermark.*) FROM change_log_witness;`, Watermark{})

	// TODO (stickupkid): This needs to be swapped out for the following query
	// once we have a way to use functions in columns.
	// SELECT COUNT(*) AS &Result.count FROM change_log WHERE created_at > $M.created_at LIMIT $M.limit;
	selectChangeLogQuery = sqlair.MustPrepare(`SELECT id AS &ChangeLog.id FROM change_log WHERE created_at > $M.created_at LIMIT $M.limit;`, ChangeLog{}, sqlair.M{})
)

func (w *Pruner) locateLowestWatermark(ctx context.Context, tx *sqlair.TX, namespace string) (Watermark, error) {
	// Gather all the valid watermarks, post row pruning. These include
	// the controller id which we know are valid based on the
	// controller_node table. If at any point we delete rows from the
	// change_log_witness table, the change stream will put the witness
	// back in place after the next change log is written.
	var watermarks []Watermark
	if err := tx.Query(ctx, selectWitnessQuery).GetAll(&watermarks); err != nil {
		return Watermark{}, errors.Trace(err)
	}

	// Nothing to do if there are no watermarks.
	if len(watermarks) == 0 {
		return Watermark{}, nil
	}

	// Gather all the watermarks that are within the windowed time period.
	// If there are no watermarks within the window, then we can assume
	// that the stream is keeping up and we don't need to prune anything.
	lowest, ok := w.lowestWatermark(watermarks, w.cfg.Clock.Now())
	if !ok {
		// Check to see if the latest change log has a valid log in the last
		// window duration, if not, then we can assume that the stream is not
		// keeping up and we should log a warning.
		var changes []ChangeLog
		if err := tx.Query(ctx, selectChangeLogQuery, sqlair.M{
			"created_at": w.cfg.Clock.Now().Add(-defaultWindowDuration),
			"limit":      changestream.DefaultNumTermWatermarks + 1,
		}).GetAll(&changes); err != nil {
			return Watermark{}, errors.Trace(err)
		}
		// If there are less than the default number of term watermarks, then
		// we can assume that the stream is keeping up and we don't need to
		// prune anything.
		if len(changes) < changestream.DefaultNumTermWatermarks {
			return Watermark{}, nil
		}
		w.cfg.Logger.Warningf("No watermarks within window, check logs to see if the change stream is keeping up")
		return Watermark{}, nil
	}
	return lowest, nil
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

func (w *Pruner) lowestWatermark(watermarks []Watermark, now time.Time) (Watermark, bool) {
	// Select the lower bound of the watermark, only if the updated_at time
	// is within a windowed time period.
	var (
		view = window{
			start: now.Add(-defaultWindowDuration),
			end:   now,
		}
		lowest = Watermark{
			LowerBound: math.MaxInt64,
		}
	)
	for _, watermark := range watermarks {
		// TODO (stickupkid): If any watermarks are outside of the window, then
		// we should force the falling behind controller to bounce and to try
		// again at keeping up. We don't have any mechanism to do this yet,
		// adding a item to table might be a waste of time. If the controller
		// isn't inserting into the change log, they're either bouncing all the
		// time or are deadlocked. If they're the latter no amount of inserting
		// into a table will solve the problem, as they're not reading the
		// change log anyway.
		// In addition to this, we have no theatre experience about what a
		// good valid window time is. For now we'll just log a warning for
		// visibility, before we solidify the approach.
		if !view.contains(watermark.UpdatedAt) {
			w.cfg.Logger.Warningf("Watermark %q is outside of window, check logs to see if the change stream is keeping up", watermark.ControllerID)
		}

		// Select the lower bound of the watermark.
		if watermark.LowerBound < lowest.LowerBound {
			lowest = watermark
		}
	}

	// Nothing was selected, this means that there are no watermarks within
	// the windowed time period. It could also mean that potentially there are
	// are now controllers not keeping up or recording their changes.
	if lowest.LowerBound == math.MaxInt64 {
		return Watermark{}, false
	}

	return lowest, true
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *Pruner) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

// Model represents a model from the model_list table.
type Model struct {
	UUID string `db:"uuid"`
}

// Watermark represents a row from the change_log_witness table.
type Watermark struct {
	ControllerID string    `db:"controller_id"`
	LowerBound   int64     `db:"lower_bound"`
	UpdatedAt    time.Time `db:"updated_at"`
}
