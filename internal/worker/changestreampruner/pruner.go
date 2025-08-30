// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"
	"sort"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
)

type modelPruner struct {
	tomb tomb.Tomb

	db        coredatabase.TxnRunner
	namespace string

	window       window
	updateWindow WindowUpdaterFunc

	clock  clock.Clock
	logger logger.Logger
}

// NewModelPruner creates a new modelPruner for the given database and
// namespace.
func NewModelPruner(
	db coredatabase.TxnRunner,
	namespace string,
	window window,
	clock clock.Clock,
	logger logger.Logger,
) *modelPruner {
	w := &modelPruner{
		db:        db,
		namespace: namespace,

		window: window,

		clock:  clock,
		logger: logger,
	}

	w.tomb.Go(w.loop)
	return w
}

func (w *modelPruner) loop() error {
	ctx := w.tomb.Context(context.Background())

	if err := w.db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Locate the lowest watermark, this is the watermark that we will
		// use to prune the change log.
		lowest, window, err := w.locateLowestWatermark(ctx, tx)
		if err != nil {
			return errors.Annotatef(err, "failed to locate lowest watermark for namespace %s", w.namespace)
		}

		// Update the current window, this is used to determine if the
		// change stream is keeping up with the pruner.
		w.updateWindow(window)

		// Prune the change log, using the lowest watermark.
		pruned, err := w.deleteChangeLog(ctx, tx, lowest)
		if err != nil {
			return errors.Annotatef(err, "failed to prune change log")
		}
		if pruned > 0 {
			w.logger.Infof(ctx, "pruned %d rows from change log", pruned)
		}
		return nil
	}); err != nil {
		return errors.Trace(err)
	}

	return nil
}

var selectWitnessQuery = sqlair.MustPrepare(`SELECT (controller_id, lower_bound, updated_at) AS (&Watermark.*) FROM change_log_witness;`, Watermark{})

func (w *modelPruner) locateLowestWatermark(ctx context.Context, tx *sqlair.TX) (Watermark, window, error) {
	// Gather all the valid watermarks, post row pruning. These include
	// the controller id which we know are valid based on the
	// controller_node table. If at any point we delete rows from the
	// change_log_witness table, the change stream will put the witness
	// back in place after the next change log is written.
	var watermarks []Watermark
	if err := tx.Query(ctx, selectWitnessQuery).GetAll(&watermarks); errors.Is(err, sqlair.ErrNoRows) {
		// Nothing to do if there are no watermarks.
		return Watermark{}, window{}, nil
	} else if err != nil {
		return Watermark{}, window{}, errors.Trace(err)
	}

	// Gather all the watermarks that are within the windowed time period.
	// If there are no watermarks within the window, then we can assume
	// that the stream is keeping up and we don't need to prune anything.
	sorted := sortWatermarks(watermarks)

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
	now := w.clock.Now()
	timeView := window{
		start: now.Add(-defaultWindowDuration),
		end:   now,
	}
	if !timeView.Contains(watermarkView) && !watermarkView.Equals(w.window) {
		w.logger.Warningf(ctx, "watermarks %q are outside of window, check logs to see if the change stream is keeping up", sorted[0].ControllerID)
	}

	return sorted[0], watermarkView, nil
}

var deleteQuery = sqlair.MustPrepare(`DELETE FROM change_log WHERE id <= $M.id;`, sqlair.M{})

func (w *modelPruner) deleteChangeLog(ctx context.Context, tx *sqlair.TX, lowest Watermark) (int64, error) {
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
	return w.catacomb.Context(ctx), cancel
}

func sortWatermarks(watermarks []Watermark) []Watermark {
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
