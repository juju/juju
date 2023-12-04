// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/gate"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// WorkerConfig encapsulates the configuration options for the
// bootstrap worker.
type WorkerConfig struct {
	Agent             agent.Agent
	ObjectStore       objectstore.ObjectStore
	BootstrapUnlocker gate.Unlocker
	AgentBinarySeeder AgentBinaryBootstrapFunc

	// Deprecated: This is only here, until we can remove the state layer.
	State *state.State

	Logger Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if c.ObjectStore == nil {
		return errors.NotValidf("nil ObjectStore")
	}
	if c.BootstrapUnlocker == nil {
		return errors.NotValidf("nil BootstrapUnlocker")
	}
	if c.AgentBinarySeeder == nil {
		return errors.NotValidf("nil AgentBinarySeeder")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.State == nil {
		return errors.NotValidf("nil State")
	}
	return nil
}

type bootstrapWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	tomb           tomb.Tomb
}

// NewWorker creates a new bootstrap worker.
func NewWorker(cfg WorkerConfig) (*bootstrapWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*bootstrapWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &bootstrapWorker{
		internalStates: internalStates,
		cfg:            cfg,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Kill stops the worker.
func (w *bootstrapWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop and then returns the reason it was
// killed.
func (w *bootstrapWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *bootstrapWorker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	// Perform the bootstrap.
	//  1. Read bootstrap-params
	//  2. Access the object store
	//  3. Populate tools
	//  4. Deploy the controller charm using state
	w.cfg.BootstrapUnlocker.Unlock()
	return nil
}

func (w *bootstrapWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
