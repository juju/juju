// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dualwritehack

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/internal/worker"
)

// NewWorker returns a worker.Worker for DualWriteWorker.
func NewWorker(config Config) (*DualWriteWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &DualWriteWorker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.init,
	})
	return w, errors.Trace(err)
}

// DualWriteWorker prunes status history or action records at regular intervals.
type DualWriteWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is defined on worker.Worker.
func (w *DualWriteWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *DualWriteWorker) Wait() error {
	return w.catacomb.Wait()
}

// Catacomb returns the prune worker's catacomb.
func (w *DualWriteWorker) Catacomb() *catacomb.Catacomb {
	return &w.catacomb
}

// Config return the prune worker's config.
func (w *DualWriteWorker) Config() *Config {
	return &w.config
}

func (w *DualWriteWorker) init() error {
	if w.config.StatePool == nil {
		w.config.Logger.Infof("no state pool, doing nothing")
		<-w.catacomb.Dying()
		return w.catacomb.ErrDying()
	}

	// Add your dual write workers here.
	modelConfigDualWriteWorker := worker.NewSimpleWorker(w.ModelConfigDualWrite)
	if err := w.catacomb.Add(modelConfigDualWriteWorker); err != nil {
		return errors.Trace(err)
	}
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

// ModelConfigDualWrite is a worker that watches dqlite model config and smashes it into
// mongo state model config.
func (w *DualWriteWorker) ModelConfigDualWrite(stopCh <-chan struct{}) error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	modelConfigService := w.config.ServiceFactory.Config()
	modelConfig, err := modelConfigService.ModelConfig(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot load model configuration")
	}

	st, err := w.config.StatePool.Get(modelConfig.UUID())
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	modelConfigWatcher, err := modelConfigService.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigWatcher.Kill()

	for {
		select {
		case <-stopCh:
			return nil

		case _, ok := <-modelConfigWatcher.Changes():
			if !ok {
				return errors.New("model configuration watcher closed")
			}
			modelConfig, err := modelConfigService.ModelConfig(ctx)
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration from dqlite")
			}
			configToSmashIn := modelConfig.AllAttrs()
			err = m.ForceUpdateModelConfigForDualWrite(configToSmashIn)
			if err != nil {
				return errors.Annotate(err, "cannot force write model config to state from dqlite")
			}
			w.config.Logger.Infof("updated model config in mongo")
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *DualWriteWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}
