// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/worker/gate"
)

// WorkerConfig encapsulates the configuration options for the
// bootstrap worker.
type WorkerConfig struct {
	ObjectStore       objectstore.ObjectStore
	BootstrapUnlocker gate.Unlocker
	Logger            Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.ObjectStore == nil {
		return errors.NotValidf("nil ObjectStore")
	}
	if c.BootstrapUnlocker == nil {
		return errors.NotValidf("nil BootstrapUnlocker")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
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
	loggo.GetLogger("***").Criticalf("bootstrapWorker loop")
	// TODO (stickupkid): Upload the charm.
	w.cfg.BootstrapUnlocker.Unlock()
	return tomb.ErrDying
}
