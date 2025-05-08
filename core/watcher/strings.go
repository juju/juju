// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// StringsChannel is a channel that receives a baseline set of values, and
// subsequent values representing additions, changes, and/or removals of those
// values.
// This is deprecated; use <-chan []string instead.
type StringsChannel = <-chan []string

// StringsWatcher sends a single value indicating a baseline set of values, and
// subsequent values representing additions, changes, and/or removals of those
// values.
type StringsWatcher = Watcher[[]string]

// StringsHandler defines the operation of a StringsWorker.
type StringsHandler interface {

	// SetUp is called once when creating a StringsWorker. It must return a
	// StringsWatcher or an error. The StringsHandler takes responsibility for
	// stopping any returned watcher and handling any errors.
	SetUp(ctx context.Context) (StringsWatcher, error)

	// Handle is called with every value received from the StringsWatcher
	// returned by SetUp. If it returns an error, the StringsWorker will be
	// stopped.
	//
	// If Handle runs any blocking operations it must pass through, or select
	// on, the supplied abort channel; this channel will be closed when the
	// StringsWorker is killed. An aborted Handle should not return an error.
	Handle(ctx context.Context, changes []string) error

	// TearDown is called once when stopping a StringsWorker, whether or not
	// SetUp succeeded. It need not concern itself with the StringsWatcher, but
	// must clean up any other resources created in SetUp or Handle.
	TearDown() error
}

// StringsConfig holds the direct dependencies of a StringsWorker.
type StringsConfig struct {
	Handler StringsHandler
}

// Validate returns ann error if the config cannot start a StringsWorker.
func (config StringsConfig) Validate() error {
	if config.Handler == nil {
		return errors.Errorf("nil Handler %w", coreerrors.NotValid)
	}
	return nil
}

// NewStringsWorker starts a new worker that runs a StringsHandler.
func NewStringsWorker(config StringsConfig) (*StringsWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	sw := &StringsWorker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "strings-watcher",
		Site: &sw.catacomb,
		Work: sw.loop,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return sw, nil
}

// StringsWorker is a worker that wraps a StringsWatcher.
type StringsWorker struct {
	config   StringsConfig
	catacomb catacomb.Catacomb
}

func (sw *StringsWorker) loop() (err error) {
	ctx, cancel := sw.scopedContext()
	defer cancel()

	changes := sw.setUp()
	defer sw.tearDown(err)

	for {
		select {
		case <-sw.catacomb.Dying():
			return sw.catacomb.ErrDying()
		case strings, ok := <-changes:
			if !ok {
				return errors.New("change channel closed")
			}
			err = sw.config.Handler.Handle(ctx, strings)
			if err != nil {
				return err
			}
		}
	}
}

// setUp calls the handler's SetUp method; registers any returned watcher with
// the worker's catacomb; and returns the watcher's changes channel. Any errors
// encountered kill the worker and cause a nil channel to be returned.
func (sw *StringsWorker) setUp() <-chan []string {
	ctx, cancel := sw.scopedContext()
	defer cancel()

	watcher, err := sw.config.Handler.SetUp(ctx)
	if err != nil {
		sw.catacomb.Kill(err)
	}
	if watcher == nil {
		sw.catacomb.Kill(errors.New("handler returned nil watcher"))
	} else {
		if err := sw.catacomb.Add(watcher); err != nil {
			sw.catacomb.Kill(err)
		} else {
			return watcher.Changes()
		}
	}
	return nil
}

// tearDown kills the worker with the supplied error; and then kills it with
// any error returned by the handler's TearDown method.
func (sw *StringsWorker) tearDown(err error) {
	sw.catacomb.Kill(err)
	err = sw.config.Handler.TearDown()
	sw.catacomb.Kill(err)
}

// Kill is part of the worker.Worker interface.
func (sw *StringsWorker) Kill() {
	sw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (sw *StringsWorker) Wait() error {
	return sw.catacomb.Wait()
}

// Report implements dependency.Reporter.
func (sw *StringsWorker) Report() map[string]interface{} {
	if r, ok := sw.config.Handler.(worker.Reporter); ok {
		return r.Report()
	}
	return nil
}

func (sw *StringsWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(sw.catacomb.Context(context.Background()))
}
