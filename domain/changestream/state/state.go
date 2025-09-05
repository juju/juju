// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"sort"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/changestream"
)

const (
	// defaultWindowDuration is the default duration of the window in which
	// the pruner will select the lower bound of the watermark. If any
	// watermarks are outside of this window, they will not be selected and the
	// pruner will discard those watermarks.
	defaultWindowDuration = time.Minute * 10
)

// State represents database interactions dealing with the changestream.
type State struct {
	*domain.StateBase

	logger logger.Logger
	clock  clock.Clock
}

// NewState returns a new changestream state based on the input database factory
// method.
func NewState(factory coredatabase.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// Prune prunes the change log up to the lowest watermark across all
// controllers. It returns the number of rows pruned.
func (st *State) Prune(ctx context.Context, currentWindow changestream.Window) (changestream.Window, int64, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return changestream.Window{}, -1, errors.Trace(err)
	}

	var (
		pruned    int64
		newWindow changestream.Window
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Locate the lowest watermark, this is the watermark that we will
		// use to prune the change log.
		lowest, window, err := st.locateLowestWatermark(ctx, tx, currentWindow)
		if err != nil {
			return errors.Annotatef(err, "failed to locate lowest watermark")
		}

		// Update the current window, this is used to determine if the
		// change stream is keeping up with the pruner.
		newWindow = window

		// Prune the change log, using the lowest watermark.
		pruned, err = st.deleteChangeLog(ctx, tx, lowest)
		if err != nil {
			return errors.Annotatef(err, "failed to prune change log")
		}

		return nil
	})
	return newWindow, pruned, errors.Trace(err)
}

func (st *State) locateLowestWatermark(ctx context.Context, tx *sqlair.TX, current changestream.Window) (Watermark, changestream.Window, error) {
	selectWitnessQuery, err := st.Prepare(`SELECT &Watermark.* FROM change_log_witness;`, Watermark{})
	if err != nil {
		return Watermark{}, changestream.Window{}, errors.Trace(err)
	}

	// Gather all the valid watermarks, post row pruning. These include
	// the controller id which we know are valid based on the
	// controller_node table. If at any point we delete rows from the
	// change_log_witness table, the change stream will put the witness
	// back in place after the next change log is written.
	var watermarks []Watermark
	if err := tx.Query(ctx, selectWitnessQuery).GetAll(&watermarks); errors.Is(err, sqlair.ErrNoRows) {
		// Nothing to do if there are no watermarks.
		return Watermark{}, changestream.Window{}, nil
	} else if err != nil {
		return Watermark{}, changestream.Window{}, errors.Trace(err)
	}

	// Gather all the watermarks that are within the windowed time period.
	// If there are no watermarks within the window, then we can assume
	// that the stream is keeping up and we don't need to prune anything.
	sorted := sortWatermarks(watermarks)

	// Find the first and last watermark in the sorted list, this is our
	// window. It should hold the start and the end of the window.
	watermarkView := changestream.Window{
		Start: sorted[0].UpdatedAt,
		End:   sorted[len(sorted)-1].UpdatedAt,
	}

	// If the watermark is outside of the window, we should log a warning
	// message to indicate that the change stream is not keeping up. Only if
	// the watermark is different from the last window, as we don't want to
	// spam the logs if there are no changes.
	now := st.clock.Now()
	timeView := changestream.Window{
		Start: now.Add(-defaultWindowDuration),
		End:   now,
	}

	if !timeView.Contains(watermarkView) && !watermarkView.Equals(current) {
		st.logger.Warningf(ctx, "watermarks %q are outside of window, check logs to see if the change stream is keeping up", sorted[0].ControllerID)
	}

	return sorted[0], watermarkView, nil
}

func (st *State) deleteChangeLog(ctx context.Context, tx *sqlair.TX, lowest Watermark) (int64, error) {
	deleteQuery, err := st.Prepare(`DELETE FROM change_log WHERE id <= $M.id;`, sqlair.M{})
	if err != nil {
		return -1, errors.Trace(err)
	}

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
