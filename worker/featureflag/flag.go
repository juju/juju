// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featureflag

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

// This implements a flag worker that restarts any time the specified
// feature flag is turned on or off, and exposes whether the the
// feature flag is set in its Check method. (The sense of the feature
// flag can be inverted if needed, so that Check returns true when it
// is off.)

// ConfigSource lets us get notifications of changes to controller
// configuration, and then get the changed config. (Primary
// implementation is State.)
type ConfigSource interface {
	WatchControllerConfig() state.NotifyWatcher
	ControllerConfig() (controller.Config, error)
}

// ErrRefresh indicates that the flag's Check result is no longer valid,
// and a new featureflag.Worker must be started to get a valid result.
var ErrRefresh = errors.New("feature flag changed, restart worker")

// Config holds the information needed by the featureflag worker.
type Config struct {
	Source   ConfigSource
	Logger   loggo.Logger
	FlagName string
	Invert   bool
}

// Value returns whether the feature flag is set (inverted if
// necessary).
func (config Config) Value() (bool, error) {
	controllerConfig, err := config.Source.ControllerConfig()
	if err != nil {
		return false, errors.Annotate(err, "getting controller config")
	}
	flagSet := controllerConfig.Features().Contains(config.FlagName)
	return flagSet != config.Invert, nil
}

// Worker implements worker.Worker and util.Flag, representing
// controller ownership of a model, such that the Flag's validity is
// tied to the Worker's lifetime.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	value    bool
}

// NewWorker creates a feature flag worker with the specified config.
func NewWorker(config Config) (worker.Worker, error) {
	value, err := config.Value()
	if err != nil {
		return nil, errors.Trace(err)
	}
	flag := &Worker{
		config: config,
		value:  value,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &flag.catacomb,
		Work: flag.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return flag, nil
}

// Kill is part of the worker.Worker interface.
func (flag *Worker) Kill() {
	flag.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (flag *Worker) Wait() error {
	return flag.catacomb.Wait()
}

// Check is part of the util.Flag interface.
//
// Check returns whether the feature flag is set (or the not set, if
// Invert was set).
//
// The validity of this result is tied to the lifetime of the Worker;
// once the worker has stopped, no inferences may be drawn from any
// Check result.
func (flag *Worker) Check() bool {
	return flag.value
}

func (flag *Worker) loop() error {
	watcher := flag.config.Source.WatchControllerConfig()
	if err := flag.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-flag.catacomb.Dying():
			return flag.catacomb.ErrDying()
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.Errorf("watcher channel closed")
			}
			newValue, err := flag.config.Value()
			if err != nil {
				return errors.Trace(err)
			}
			if newValue != flag.value {
				flag.config.Logger.Debugf("feature flag changed: %v", newValue)
				return ErrRefresh
			}
		}
	}
}
