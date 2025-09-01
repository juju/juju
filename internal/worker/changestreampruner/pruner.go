// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"
	"fmt"
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

	db coredatabase.TxnRunner

	namespaceWindow NamespaceWindow

	clock  clock.Clock
	logger logger.Logger
}

// NewModelPruner creates a new modelPruner for the given database and
// namespace.
func NewModelPruner(
	db coredatabase.TxnRunner,
	namespaceWindow NamespaceWindow,
	clock clock.Clock,
	logger logger.Logger,
) *modelPruner {
	w := &modelPruner{
		db: db,

		namespaceWindow: namespaceWindow,

		clock:  clock,
		logger: logger,
	}

	w.tomb.Go(w.loop)
	return w
}

// Kill stops the model pruner.
func (w *modelPruner) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the model pruner to stop.
func (w *modelPruner) Wait() error {
	return w.tomb.Wait()
}

// Report returns the current status of the model pruner.
func (w *modelPruner) Report() map[string]any {
	return map[string]any{
		"namespace": w.namespaceWindow.Namespace(),
	}
}

func (w *modelPruner) loop() error {
	ctx := w.tomb.Context(context.Background())
	pruned, err := w.prune(ctx)
	if errors.Is(err, context.Canceled) {
		return tomb.ErrDying
	} else if err != nil {
		return errors.Trace(err)
	}
	if pruned > 0 {
		w.logger.Infof(ctx, "pruned %d rows from change log", pruned)
	}
	return nil
}

func (w *modelPruner) prune(ctx context.Context) (int64, error) {
	currentWindow := w.namespaceWindow.CurrentWindow()

	var pruned int64
	err := w.db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Locate the lowest watermark, this is the watermark that we will
		// use to prune the change log.
		lowest, window, err := w.locateLowestWatermark(ctx, tx, currentWindow)
		if err != nil {
			return errors.Annotatef(err, "failed to locate lowest watermark")
		}

		// Update the current window, this is used to determine if the
		// change stream is keeping up with the pruner.
		w.namespaceWindow.UpdateWindow(window)

		// Prune the change log, using the lowest watermark.
		pruned, err = w.deleteChangeLog(ctx, tx, lowest)
		if err != nil {
			return errors.Annotatef(err, "failed to prune change log")
		}

		return nil
	})
	return pruned, errors.Trace(err)
}

var selectWitnessQuery = sqlair.MustPrepare(`SELECT &Watermark.* FROM change_log_witness;`, Watermark{})

func (w *modelPruner) locateLowestWatermark(ctx context.Context, tx *sqlair.TX, current Window) (Watermark, Window, error) {
	// Gather all the valid watermarks, post row pruning. These include
	// the controller id which we know are valid based on the
	// controller_node table. If at any point we delete rows from the
	// change_log_witness table, the change stream will put the witness
	// back in place after the next change log is written.
	var watermarks []Watermark
	if err := tx.Query(ctx, selectWitnessQuery).GetAll(&watermarks); errors.Is(err, sqlair.ErrNoRows) {
		// Nothing to do if there are no watermarks.
		return Watermark{}, Window{}, nil
	} else if err != nil {
		return Watermark{}, Window{}, errors.Trace(err)
	}

	// Gather all the watermarks that are within the windowed time period.
	// If there are no watermarks within the window, then we can assume
	// that the stream is keeping up and we don't need to prune anything.
	sorted := sortWatermarks(watermarks)

	// Find the first and last watermark in the sorted list, this is our
	// window. It should hold the start and the end of the window.
	watermarkView := Window{
		start: sorted[0].UpdatedAt,
		end:   sorted[len(sorted)-1].UpdatedAt,
	}

	// If the watermark is outside of the window, we should log a warning
	// message to indicate that the change stream is not keeping up. Only if
	// the watermark is different from the last window, as we don't want to
	// spam the logs if there are no changes.
	now := w.clock.Now()
	timeView := Window{
		start: now.Add(-defaultWindowDuration),
		end:   now,
	}

	if !timeView.Contains(watermarkView) && !watermarkView.Equals(current) {
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
	Namespace string `db:"namespace"`
}

// Watermark represents a row from the change_log_witness table.
type Watermark struct {
	ControllerID string    `db:"controller_id"`
	LowerBound   int64     `db:"lower_bound"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// Window represents a time window with a start and end time.
type Window struct {
	start, end time.Time
}

// Contains returns true if the Window contains the given time.
func (w Window) Contains(o Window) bool {
	if w.Equals(o) {
		return true
	}
	return w.start.Before(o.start) && w.end.After(o.end)
}

// Equals returns true if the Window is equal to the given Window.
func (w Window) Equals(o Window) bool {
	return w.start.Equal(o.start) && w.end.Equal(o.end)
}

func (w Window) String() string {
	return fmt.Sprintf("start: %s, end: %s", w.start.Format(time.RFC3339), w.end.Format(time.RFC3339))
}
