// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/catacomb"
)

// StringsChannel is a change channel as described in the CoreWatcher docs.
//
// It sends a single value indicating a baseline set of values, and subsequent
// values representing additions, changes, and/or removals of those values. The
// precise semantics may depend upon the individual watcher.
type StringsChannel <-chan []string

// StringsWatcher conveniently ties a StringsChannel to the worker.Worker that
// represents its validity.
type StringsWatcher interface {
	CoreWatcher
	Changes() StringsChannel
}

// StringsHandler defines the operation of a StringsWorker.
type StringsHandler interface {

	// SetUp is called once when creating a StringsWorker. It must return a
	// StringsWatcher or an error. The StringsHandler takes responsibility for
	// stopping any returned watcher and handling any errors.
	SetUp() (StringsWatcher, error)

	// Handle is called with every value received from the StringsWatcher
	// returned by SetUp. If it returns an error, the StringsWorker will be
	// stopped.
	//
	// If Handle runs any blocking operations it must pass through, or select
	// on, the supplied abort channel; this channel will be closed when the
	// StringsWorker is killed. An aborted Handle should not return an error.
	Handle(abort <-chan struct{}, changes []string) error

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
		return errors.NotValidf("nil Handler")
	}
	return nil
}

// NewStringsWorker starts a new worker that runs a StringsHandler.
func NewStringsWorker(config StringsConfig) (*StringsWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	sw := &StringsWorker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &sw.catacomb,
		Work: sw.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sw, nil
}

// StringsWorker is a worker that wraps a StringsWatcher.
type StringsWorker struct {
	config   StringsConfig
	catacomb catacomb.Catacomb
}

func (sw *StringsWorker) loop() (err error) {
	changes := sw.setUp()
	defer sw.tearDown(err)
	abort := sw.catacomb.Dying()
	for {
		select {
		case <-abort:
			return sw.catacomb.ErrDying()
		case strings, ok := <-changes:
			if !ok {
				return errors.New("change channel closed")
			}
			err = sw.config.Handler.Handle(abort, strings)
			if err != nil {
				return err
			}
		}
	}
}

// setUp calls the handler's SetUp method; registers any returned watcher with
// the worker's catacomb; and returns the watcher's changes channel. Any errors
// encountered kill the worker and cause a nil channel to be returned.
func (sw *StringsWorker) setUp() StringsChannel {
	watcher, err := sw.config.Handler.SetUp()
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
