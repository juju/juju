// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	stdcontext "context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/worker/common"
)

// Facade covers the parts of the api/undertaker.UndertakerClient that we
// need for the worker. It's more than a little raw, but we'll survive.
type Facade interface {
	ModelInfo() (params.UndertakerModelInfoResult, error)
	WatchModelResources() (watcher.NotifyWatcher, error)
	ProcessDyingModel() error
	RemoveModel() error
	SetStatus(status status.Status, message string, data map[string]interface{}) error
}

// Logger defines a way to report non-fatal errors.
type Logger interface {
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// Config holds the resources and configuration necessary to run an
// undertaker worker.
type Config struct {
	Facade        Facade
	Destroyer     environs.CloudDestroyer
	CredentialAPI common.CredentialAPI
	Logger        Logger
	Clock         clock.Clock
}

// Validate returns an error if the config cannot be expected to drive
// a functional undertaker worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.CredentialAPI == nil {
		return errors.NotValidf("nil CredentialAPI")
	}
	if config.Destroyer == nil {
		return errors.NotValidf("nil Destroyer")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewUndertaker returns a worker which processes a dying model.
func NewUndertaker(config Config) (*Undertaker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	u := &Undertaker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.run,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

type Undertaker struct {
	catacomb catacomb.Catacomb
	config   Config

	callCtx context.ProviderCallContext
}

// Kill is part of the worker.Worker interface.
func (u *Undertaker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *Undertaker) Wait() error {
	return u.catacomb.Wait()
}

func (u *Undertaker) run() error {
	result, err := u.config.Facade.ModelInfo()
	// If model already gone, exit early.
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	modelInfo := result.Result

	u.config.Logger.Debugf(
		"destroying model %q with timeout %v, force=%v",
		modelInfo.Name, modelInfo.DestroyTimeout, modelInfo.ForceDestroyed)

	if modelInfo.Life == life.Alive {
		return errors.Errorf("model still alive")
	}
	if modelInfo.Life == life.Dying {
		// TODO(axw) 2016-04-14 #1570285
		// We should update status with information
		// about the remaining resources here, and
		// also make the worker responsible for
		// checking the emptiness criteria before
		// attempting to remove the model.
		if err := u.setStatus(
			status.Destroying,
			"cleaning up cloud resources",
		); err != nil {
			return errors.Trace(err)
		}
		// Process the dying model. This blocks until the model
		// is dead, the timeout expires, or the worker is stopped.
		if err := u.processDyingModel(modelInfo.DestroyTimeout); err != nil {
			if !errors.IsTimeout(err) || !modelInfo.ForceDestroyed {
				return errors.Trace(err)
			}
		}
	}

	if modelInfo.IsSystem {
		// Nothing to do. We don't destroy environ resources or
		// delete model docs for a controller model, because we're
		// running inside that controller and can't safely clean up
		// our own infrastructure. (That'll be the client's job in
		// the end, once we've reported that we've tidied up what we
		// can, by returning nil here, indicating that we've set it
		// to Dead -- implied by processDyingModel succeeding.)
		return nil
	}

	if err := u.destroyEnviron(modelInfo); err != nil {
		// If forced and destroy timeout is zero, don't exit, continue to remove the model.
		if !modelInfo.ForceDestroyed || !errors.IsTimeout(err) ||
			(modelInfo.DestroyTimeout != nil && *modelInfo.DestroyTimeout > 0) {
			return errors.Annotate(err, "cannot destroy cloud resources")
		}
	}

	// Finally, the model is going to be dead, and be removed.
	if err := u.config.Facade.RemoveModel(); err != nil {
		return errors.Annotate(err, "cannot remove model")
	}
	return nil
}

func (u *Undertaker) destroyEnviron(modelInfo params.UndertakerModelInfo) error {
	timeout := modelInfo.DestroyTimeout
	if timeout != nil && *timeout == 0 {
		return errors.Timeoutf("destroy model")
	}
	u.config.Logger.Debugf("destroying cloud resources for model %v", modelInfo.Name)
	// Now the model is known to be hosted and dying, we can tidy up any
	// provider resources it might have used.
	if err := u.setStatus(
		status.Destroying, "tearing down cloud environment",
	); err != nil {
		return errors.Trace(err)
	}

	// We may or may not have been given a timeout to use.
	var (
		ctx    stdcontext.Context
		cancel func()
	)
	if timeout != nil {
		ctx, cancel = stdcontext.WithTimeout(stdcontext.Background(), *timeout)
	} else {
		ctx, cancel = stdcontext.WithCancel(stdcontext.Background())
	}
	defer cancel()
	callCtx := common.NewCloudCallContextFunc(u.config.CredentialAPI)(ctx)

	errChan := make(chan error, 1)
	go func() {
		errChan <- u.config.Destroyer.Destroy(callCtx)
	}()

	var destroyErr error
	select {
	case <-u.catacomb.Dying():
		return nil
	case destroyErr = <-errChan:
	case <-ctx.Done():
		destroyErr = ctx.Err()
		if destroyErr == stdcontext.DeadlineExceeded {
			destroyErr = errors.Timeoutf("destroy model")
		}
	}
	if destroyErr != nil {
		// If the destroy is forced, log the error and return nil.
		// However, if the error was a timeout, that error is still returned
		// so it can be processed by the caller.
		if modelInfo.ForceDestroyed {
			u.config.Logger.Errorf(
				"error tearing down cloud environment for force-destroyed model %q (%s): %v",
				modelInfo.GlobalName, modelInfo.UUID, destroyErr)
			if !errors.IsTimeout(destroyErr) {
				destroyErr = nil
			}
		}
	}
	return errors.Trace(destroyErr)
}

func (u *Undertaker) setStatus(modelStatus status.Status, message string) error {
	return u.config.Facade.SetStatus(modelStatus, message, nil)
}

func (u *Undertaker) processDyingModel(timeout *time.Duration) error {
	if timeout != nil && *timeout == 0 {
		return errors.Timeoutf("process dying model")
	}
	// Get the timeout here so tests can manipulate the clock as needed.
	var timeoutAfter <-chan time.Time
	if timeout != nil {
		timeoutAfter = u.config.Clock.After(*timeout)
	}

	watcher, err := u.config.Facade.WatchModelResources()
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	defer watcher.Kill()
	attempt := 1
	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case <-timeoutAfter:
			return errors.Timeoutf("process dying model")
		case <-watcher.Changes():
			err := u.config.Facade.ProcessDyingModel()
			if err == nil {
				// ProcessDyingModel succeeded. We're free to
				// destroy any remaining environ resources.
				return nil
			}
			if !params.IsCodeModelNotEmpty(err) && !params.IsCodeHasHostedModels(err) {
				return errors.Trace(err)
			}
			// Retry once there are changes to the model's resources.
			_ = u.setStatus(
				status.Destroying,
				fmt.Sprintf("attempt %d to destroy model failed (will retry):  %v", attempt, err),
			)
		}
		attempt++
	}
}
