// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/multiwatcher"
)

type control interface {
	Dying() <-chan struct{}
	Err() error
}

// Watcher watches any changes to the state.
type Watcher struct {
	request chan *request
	control control
	logger  logger.Logger
	err     chan error

	filter func([]multiwatcher.Delta) []multiwatcher.Delta

	// The following fields are maintained by the Worker goroutine.
	revno   int64
	stopped bool

	// used indicates that the watcher was used (i.e. Next() called).
	used bool
}

func (w *Watcher) Wait() error {
	return nil
}

func (w *Watcher) Kill() {
	select {
	case w.request <- &request{watcher: w}:
		// We asked to be stopped, and this is processed.
	case _, stillOpen := <-w.err:
		// We have been stopped already.
		if stillOpen {
			close(w.err)
		}
		// Since we are asking to stop, we don't care about the underlying
		// error from the read.
	case <-w.control.Dying():
		// The worker is dying and we will be cleaned up.
	}
	// In all of the above cases, we're fine, and don't need to return an
	// error that would be logged.
}

// Next retrieves all changes that have happened since the last
// time it was called, blocking until there are some changes available.
//
// The result from the initial call to Next() is different from
// subsequent calls. The latter will reflect changes that have happened
// since the last Next() call. In contrast, the initial Next() call will
// return the deltas that represent the model's complete state at that
// moment, even when the model is empty. In that empty model case an
// empty set of deltas is returned.
func (w *Watcher) Next(ctx context.Context) ([]multiwatcher.Delta, error) {
	// In order to be able to apply the filter, yet not signal the caller when
	// all deltas were filtered out, we need an outer loop.
	var changes []multiwatcher.Delta
	for len(changes) == 0 {
		req := &request{
			watcher: w,
			reply:   make(chan bool),
		}
		if !w.used {
			req.noChanges = make(chan struct{})
			w.used = true
		}

		select {
		case err, open := <-w.err:
			if open {
				w.logger.Tracef("hit an error: %v", err)
				close(w.err)
				return nil, errors.Trace(err)
			}
			w.logger.Tracef("err channel closed")
			return nil, errors.Trace(multiwatcher.NewErrStopped())
		case <-w.control.Dying():
			w.logger.Tracef("worker dying")
			err := w.control.Err()
			if err == nil {
				err = multiwatcher.ErrStoppedf("shared state watcher")
			}
			return nil, err
		case w.request <- req:
		}

		select {
		case err, open := <-w.err:
			if open {
				w.logger.Tracef("hit an error: %v", err)
				close(w.err)
				return nil, errors.Trace(err)
			}
			w.logger.Tracef("err channel closed")
			return nil, errors.Trace(multiwatcher.NewErrStopped())
		case <-w.control.Dying():
			w.logger.Tracef("worker dying")
			err := w.control.Err()
			if err == nil {
				err = multiwatcher.ErrStoppedf("shared state watcher")
			}
			return nil, err
		case ok := <-req.reply:
			if !ok {
				return nil, errors.Trace(multiwatcher.NewErrStopped())
			}
		case <-req.noChanges:
			return []multiwatcher.Delta{}, nil
		}

		changes = req.changes
		w.logger.Tracef("received %d changes", len(changes))
		if w.filter != nil {
			changes = w.filter(changes)
			if len(changes) == 0 {
				w.logger.Tracef("filtered out all changes, looping")
			}
		}
	}
	w.logger.Tracef("returning %d changes", len(changes))
	return changes, nil
}
