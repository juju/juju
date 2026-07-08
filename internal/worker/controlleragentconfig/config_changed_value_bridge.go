// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"github.com/juju/worker/v5/dependency"
)

// ConfigChangedValueBridge adapts controller config reload notifications to a
// `voyeur.Value` trigger for workers that already converge on simple
// value-change signals.
//
// The controller's reload socket fanout is exposed through ConfigWatcher,
// while some existing workers, such as logrouter, already consume a
// `voyeur.Value` to know when they should re-read current configuration.
// This bridge subscribes to the controller reload stream and sets the supplied
// value on each notification so those workers can react to controller-local
// config changes without learning about the controller reload socket or the
// ConfigWatcher contract directly.
type ConfigChangedValueBridge struct {
	catacomb      catacomb.Catacomb
	configWatcher ConfigWatcher
	configChanged *voyeur.Value
}

// NewConfigChangedValueBridge returns a worker that forwards controller reload
// notifications from configWatcher into configChanged.
func NewConfigChangedValueBridge(
	configWatcher ConfigWatcher,
	configChanged *voyeur.Value,
) worker.Worker {
	w := &ConfigChangedValueBridge{
		configWatcher: configWatcher,
		configChanged: configChanged,
	}
	_ = catacomb.Invoke(catacomb.Plan{
		Name: "controller-config-changed-value-bridge",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w
}

// ConfigChangedValueBridgeManifoldConfig defines the controller config watcher
// input and the voyeur value updated by ConfigChangedValueBridgeManifold.
type ConfigChangedValueBridgeManifoldConfig struct {
	ControllerAgentConfigName string
	ConfigChangedValue        *voyeur.Value
}

// Validate ensures that the manifold configuration is usable.
func (c ConfigChangedValueBridgeManifoldConfig) Validate() error {
	if c.ControllerAgentConfigName == "" {
		return errors.NotValidf("empty ControllerAgentConfigName")
	}
	if c.ConfigChangedValue == nil {
		return errors.NotValidf("nil ConfigChangedValue")
	}
	return nil
}

// ConfigChangedValueBridgeManifold returns a dependency manifold that wires the
// controller-agent-config watcher into a voyeur value.
func ConfigChangedValueBridgeManifold(config ConfigChangedValueBridgeManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.ControllerAgentConfigName},
		Start: func(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var configWatcher ConfigWatcher
			if err := getter.Get(config.ControllerAgentConfigName, &configWatcher); err != nil {
				return nil, errors.Trace(err)
			}
			return NewConfigChangedValueBridge(configWatcher, config.ConfigChangedValue), nil
		},
	}
}

// Kill is part of the worker.Worker interface.
func (w *ConfigChangedValueBridge) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *ConfigChangedValueBridge) Wait() error {
	return w.catacomb.Wait()
}

func (w *ConfigChangedValueBridge) loop() error {
	defer w.configWatcher.Unsubscribe()
	changes := w.configWatcher.Changes()
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-w.configWatcher.Done():
			return nil
		case _, ok := <-changes:
			if !ok {
				return nil
			}
			w.configChanged.Set(true)
		}
	}
}
