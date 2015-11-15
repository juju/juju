// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/catacomb"
)

// NotifyChan is a change channel as described in the CoreWatcher docs.
//
// It sends a single value to indicate that the watch is active, and subsequent
// values whenever the value(s) under observation change(s).
type NotifyChan <-chan struct{}

// NotifyWatcher conveniently ties a NotifyChan to the worker.Worker that
// represents its validity.
type NotifyWatcher interface {
	CoreWatcher
	Changes() NotifyChan
}

// NotifyHandler defines the operation of a NotifyWorker.
type NotifyHandler interface {

	// SetUp is called once when creating a NotifyWorker. It must return a
	// NotifyWatcher or an error. The NotifyHandler takes responsibility for
	// stopping any returned watcher and handling any errors.
	SetUp() (NotifyWatcher, error)

	// Handle is called whenever a value is received from the NotifyWatcher
	// returned by SetUp. If it returns an error, the NotifyWorker will be
	// stopped.
	//
	// If Handle runs any blocking operations it must pass through, or select
	// on, the supplied abort channel; this channnel will be closed when the
	// NotifyWorker is killed. An aborted Handle should not return an error.
	Handle(abort <-chan struct{}) error

	// TearDown is called once when stopping a NotifyWorker, whether or not
	// SetUp succeeded. It need not concern itself with the NotifyWatcher, but
	// must clean up any other resources created in SetUp or Handle.
	TearDown() error
}

// NotifyConfig holds the direct dependencies of a NotifyWorker.
type NotifyConfig struct {
	Handler NotifyHandler
}

// Validate returns an error if the config cannot start a NotifyWorker.
func (config NotifyConfig) Validate() error {
	if config.Handler == nil {
		return errors.NotValidf("nil Handler")
	}
	return nil
}

// NewNotifyWorker starts a new worker that runs a NotifyHandler.
func NewNotifyWorker(config NotifyConfig) (*NotifyWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	nw := &NotifyWorker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &nw.catacomb,
		Work: nw.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return nw, nil
}

// NotifyWorker is a worker that wraps a NotifyWatcher.
type NotifyWorker struct {
	config   NotifyConfig
	catacomb catacomb.Catacomb
}

func (nw *NotifyWorker) loop() (err error) {
	changes := nw.setUp()
	defer nw.tearDown(err)
	for {
		select {
		case <-nw.catacomb.Dying():
			return nw.catacomb.ErrDying()
		case _, ok := <-changes:
			if !ok {
				return errors.New("change channel closed")
			}
			abort := nw.catacomb.Dying()
			err = nw.config.Handler.Handle(abort)
			if err != nil {
				return err
			}
		}
	}
}

// setUp calls the handler's SetUp method; registers any returned watcher with
// the worker's catacomb; and returns the watcher's changes channel. Any errors
// encountered kill the worker and cause a nil channel to be returned.
func (nw *NotifyWorker) setUp() NotifyChan {
	watcher, err := nw.config.Handler.SetUp()
	if err != nil {
		nw.catacomb.Kill(err)
	}
	if watcher == nil {
		nw.catacomb.Kill(errors.New("handler returned nil watcher"))
	} else if err := nw.catacomb.Add(watcher); err != nil {
		nw.catacomb.Kill(err)
	} else {
		return watcher.Changes()
	}
	return nil
}

// tearDown kills the worker with the supplied error; and then kills it with
// any error returned by the handler's TearDown method.
func (nw *NotifyWorker) tearDown(err error) {
	nw.catacomb.Kill(err)
	err = nw.config.Handler.TearDown()
	nw.catacomb.Kill(err)
}

// Kill is part of the worker.Worker interface.
func (nw *NotifyWorker) Kill() {
	nw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (nw *NotifyWorker) Wait() error {
	return nw.catacomb.Wait()
}
