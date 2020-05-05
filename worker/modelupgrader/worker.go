// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	jujuworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/wrench"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

// ErrModelRemoved indicates that this worker was operating on the model that is no longer found.
var ErrModelRemoved = errors.New("model has been removed")

// Facade exposes capabilities required by the worker.
type Facade interface {
	ModelEnvironVersion(tag names.ModelTag) (int, error)
	ModelTargetEnvironVersion(tag names.ModelTag) (int, error)
	SetModelEnvironVersion(tag names.ModelTag, v int) error
	SetModelStatus(names.ModelTag, status.Status, string, map[string]interface{}) error
	WatchModelEnvironVersion(tag names.ModelTag) (watcher.NotifyWatcher, error)
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	// Facade holds the API facade used by this worker for getting,
	// setting and watching the model's environ version.
	Facade Facade

	// GateUnlocker holds a gate.Unlocker that the worker must call
	// after the model has been successfully upgraded.
	GateUnlocker gate.Unlocker

	// ControllerTag holds the tag of the controller that runs this
	// worker.
	ControllerTag names.ControllerTag

	// ModelTag holds the tag of the model to which this worker is
	// scoped.
	ModelTag names.ModelTag

	// Environ holds the Environ used to run upgrade steps, or nil
	// if the worker should wait for upgrade steps to be run by
	// another agent.
	Environ environs.Environ

	// CredentialAPI holds the API facade used to invalidate credential
	// whenever the worker makes cloud calls if credential for this model
	// becomes invalid.
	CredentialAPI common.CredentialAPI

	Logger Logger
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.GateUnlocker == nil {
		return errors.NotValidf("nil GateUnlocker")
	}
	if config.ControllerTag == (names.ControllerTag{}) {
		return errors.NotValidf("empty ControllerTag")
	}
	if config.ModelTag == (names.ModelTag{}) {
		return errors.NotValidf("empty ModelTag")
	}
	if config.CredentialAPI == nil {
		return errors.NotValidf("nil CredentialAPI")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a worker that ensures that environ/provider schema upgrades
// are run when the model is first loaded by a controller of a new version. The
// worker either runs the upgrades or waits for another controller unit to run
// them, depending on the configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	targetVersion, err := config.Facade.ModelTargetEnvironVersion(config.ModelTag)
	if err != nil {
		if params.IsCodeNotFound(err) {
			return nil, ErrModelRemoved
		}
		return nil, errors.Trace(err)
	}
	if config.Environ != nil {
		return newUpgradeWorker(config, targetVersion)
	}
	return newWaitWorker(config, targetVersion)
}

// newWaitWorker returns a worker that waits for the controller leader to run
// the upgrade steps and update the model's environ version, and then unlocks
// the gate.
func newWaitWorker(config Config, targetVersion int) (worker.Worker, error) {
	watcher, err := config.Facade.WatchModelEnvironVersion(config.ModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ww := waitWorker{
		watcher:       watcher,
		facade:        config.Facade,
		modelTag:      config.ModelTag,
		gate:          config.GateUnlocker,
		targetVersion: targetVersion,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &ww.catacomb,
		Init: []worker.Worker{watcher},
		Work: ww.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return &ww, nil
}

type waitWorker struct {
	catacomb      catacomb.Catacomb
	watcher       watcher.NotifyWatcher
	facade        Facade
	modelTag      names.ModelTag
	gate          gate.Unlocker
	targetVersion int
}

func (ww *waitWorker) Kill() {
	ww.catacomb.Kill(nil)
}

func (ww *waitWorker) Wait() error {
	return ww.catacomb.Wait()
}

func (ww *waitWorker) loop() error {
	for {
		select {
		case <-ww.catacomb.Dying():
			return ww.catacomb.ErrDying()
		case _, ok := <-ww.watcher.Changes():
			if !ok {
				return ww.catacomb.ErrDying()
			}
			currentVersion, err := ww.facade.ModelEnvironVersion(ww.modelTag)
			if err != nil {
				if params.IsCodeNotFound(err) {
					return ErrModelRemoved
				}
				return errors.Trace(err)
			}
			if currentVersion >= ww.targetVersion {
				ww.gate.Unlock()
				return nil
			}
		}
	}
}

// newUpgradeWorker returns a worker that runs the upgrade steps, updates the
// model's environ version, and unlocks the gate.
func newUpgradeWorker(config Config, targetVersion int) (worker.Worker, error) {
	currentVersion, err := config.Facade.ModelEnvironVersion(config.ModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return jujuworker.NewSimpleWorker(func(<-chan struct{}) error {
		// NOTE(axw) the abort channel is ignored, because upgrade
		// steps are not interruptible. If we find they need to be
		// interruptible, we should consider passing through a
		// context.Context for cancellation, and cancelling it if
		// the abort channel is signalled.
		setVersion := func(v int) error {
			return config.Facade.SetModelEnvironVersion(config.ModelTag, v)
		}
		setStatus := func(s status.Status, info string) error {
			return config.Facade.SetModelStatus(config.ModelTag, s, info, nil)
		}
		if targetVersion > currentVersion {
			if err := setStatus(status.Busy, fmt.Sprintf(
				"upgrading environ from version %d to %d",
				currentVersion, targetVersion,
			)); err != nil {
				return errors.Trace(err)
			}
		}
		if err := runEnvironUpgradeSteps(
			config.Environ,
			config.ControllerTag,
			config.ModelTag,
			currentVersion,
			targetVersion,
			setVersion,
			common.NewCloudCallContext(config.CredentialAPI, nil),
			config.Logger,
		); err != nil {
			info := fmt.Sprintf("failed to upgrade environ: %s", err)
			if err := setStatus(status.Error, info); err != nil {
				config.Logger.Warningf("failed to update model status: %v", err)
			}
			return errors.Annotate(err, "upgrading environ")
		}
		if err := setStatus(status.Available, ""); err != nil {
			return errors.Trace(err)
		}
		config.GateUnlocker.Unlock()
		return nil
	}), nil
}

func runEnvironUpgradeSteps(
	env environs.Environ,
	controllerTag names.ControllerTag,
	modelTag names.ModelTag,
	currentVersion int,
	targetVersion int,
	setVersion func(int) error,
	callCtx context.ProviderCallContext,
	logger Logger,
) error {
	if wrench.IsActive("modelupgrader", "fail-all") ||
		wrench.IsActive("modelupgrader", "fail-model-"+modelTag.Id()) {
		return errors.New("wrench active")
	}
	upgrader, ok := env.(environs.Upgrader)
	if !ok {
		logger.Debugf("%T does not support environs.Upgrader", env)
		return nil
	}
	args := environs.UpgradeOperationsParams{
		ControllerUUID: controllerTag.Id(),
	}
	for _, op := range upgrader.UpgradeOperations(callCtx, args) {
		if op.TargetVersion <= currentVersion {
			// The operation is for the same as or older
			// than the current environ version.
			logger.Tracef(
				"ignoring upgrade operation for version %v",
				op.TargetVersion,
			)
			continue
		}
		if op.TargetVersion > targetVersion {
			// The operation is for a version newer than
			// the provider's current version. This will
			// only happen for an improperly written provider.
			logger.Debugf(
				"ignoring upgrade operation for version %v",
				op.TargetVersion,
			)
			continue
		}
		logger.Debugf(
			"running upgrade operation for version %v",
			op.TargetVersion,
		)
		for _, step := range op.Steps {
			logger.Debugf("running step %q", step.Description())
			if err := step.Run(callCtx); err != nil {
				return errors.Trace(err)
			}
		}
		// Record the new version as we go, so we minimise the number
		// of operations we'll re-run in the case of failure.
		if err := setVersion(op.TargetVersion); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
