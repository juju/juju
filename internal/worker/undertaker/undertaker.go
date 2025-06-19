// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/retry.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/rpc/params"
)

//go:generate go run go.uber.org/mock/mockgen -package undertaker_test -destination facade_mock_test.go github.com/juju/juju/internal/worker/undertaker Facade
//go:generate go run go.uber.org/mock/mockgen -package undertaker_test -destination credentialapi_mock_test.go github.com/juju/juju/internal/worker/common CredentialAPI

// Facade covers the parts of the api/undertaker.UndertakerClient that we
// need for the worker. It's more than a little raw, but we'll survive.
type Facade interface {
	ModelConfig() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
	ModelInfo() (params.UndertakerModelInfoResult, error)
	WatchModelResources() (watcher.NotifyWatcher, error)
	WatchModel() (watcher.NotifyWatcher, error)
	ProcessDyingModel() error
	RemoveModel() error
	RemoveModelSecrets() error
	SetStatus(status status.Status, message string, data map[string]interface{}) error
}

// Logger defines a way to report non-fatal errors.
type Logger interface {
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
	Warningf(string, ...interface{})
}

// Config holds the resources and configuration necessary to run an
// undertaker worker.
type Config struct {
	Facade                Facade
	CredentialAPI         common.CredentialAPI
	Logger                Logger
	Clock                 clock.Clock
	NewCloudDestroyerFunc func(context.Context, environs.OpenParams) (environs.CloudDestroyer, error)
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
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.NewCloudDestroyerFunc == nil {
		return errors.NotValidf("nil NewCloudDestroyerFunc")
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
}

// Kill is part of the worker.Worker interface.
func (u *Undertaker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *Undertaker) Wait() error {
	return u.catacomb.Wait()
}

func (u *Undertaker) run() (errOut error) {
	defer func() {
		if errors.Is(errOut, context.Canceled) ||
			errors.Is(errOut, context.DeadlineExceeded) {
			select {
			case <-u.catacomb.Dying():
				errOut = u.catacomb.ErrDying()
			default:
			}
		}
	}()

	modelWatcher, err := u.config.Facade.WatchModel()
	if errors.Is(err, errors.NotFound) {
		// If model already gone, exit early.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	err = u.catacomb.Add(modelWatcher)
	if err != nil {
		return err
	}

	select {
	case <-modelWatcher.Changes():
	case <-u.catacomb.Dying():
		return u.catacomb.ErrDying()
	}

	result, err := u.config.Facade.ModelInfo()
	if errors.Is(err, errors.NotFound) {
		// If model already gone, exit early.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	} else if result.Error != nil {
		return errors.Trace(result.Error)
	}
	info := result.Result

	ctx, cancel := context.WithCancel(u.catacomb.Context(context.Background()))
	defer cancel()

	// Watch for changes to model destroy values, if so, cancel the context
	// and restart the worker.
	err = u.catacomb.Add(worker.NewSimpleWorker(func(stopCh <-chan struct{}) error {
		for {
			select {
			case <-stopCh:
				return nil
			case <-modelWatcher.Changes():
				result, err := u.config.Facade.ModelInfo()
				if errors.Is(err, errors.NotFound) || err != nil || result.Error != nil {
					continue
				}
				updated := result.Result
				changed := false
				switch {
				case info.DestroyTimeout == nil && updated.DestroyTimeout != nil:
					changed = true
				case info.DestroyTimeout != nil && updated.DestroyTimeout == nil:
					changed = true
				case info.DestroyTimeout != nil && updated.DestroyTimeout != nil && *info.DestroyTimeout != *updated.DestroyTimeout:
					changed = true
				case info.ForceDestroyed != updated.ForceDestroyed:
					changed = true
				}
				if changed {
					u.config.Logger.Infof("model destroy parameters changed: restarting undertaker worker")
					return errors.Errorf("model destroy parameters changed")
				}
			}
		}
	}))
	if err != nil {
		return err
	}

	if info.Life == life.Alive {
		return errors.Errorf("model still alive")
	}

	if info.ForceDestroyed && info.DestroyTimeout != nil {
		u.config.Logger.Infof("force destroying model %q with timeout %v", info.Name, info.DestroyTimeout)
		return u.forceDestroy(ctx, info)
	} else if info.DestroyTimeout != nil {
		u.config.Logger.Warningf("timeout ignored for graceful model destroy")
	}
	// Even if ForceDestroyed is true, if we don't have a timeout, we treat them the same
	// as a non-force destroyed model.
	u.config.Logger.Infof("destroying model %q", info.Name)
	return u.cleanDestroy(ctx, info)
}

func (u *Undertaker) cleanDestroy(ctx context.Context, info params.UndertakerModelInfo) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if info.Life == life.Dying {
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
		// Wait for the model to become empty.
		if err := u.processDyingModel(ctx, info); err != nil {
			u.config.Logger.Errorf("destroy model failed: %v", err)
			return fmt.Errorf("proccesing model death: %w", err)
		}
	} else {
		u.config.Logger.Debugf("skipping processDyingModel as model is already dead")
	}

	if info.IsSystem {
		// Nothing to do. We don't destroy environ resources or
		// delete model docs for a controller model, because we're
		// running inside that controller and can't safely clean up
		// our own infrastructure. (That'll be the client's job in
		// the end, once we've reported that we've tidied up what we
		// can, by returning nil here, indicating that we've set it
		// to Dead -- implied by processDyingModel succeeding.)
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	retryStrategy := retry.LimitCount(1, retry.Regular{})
	// Destroy environ resources.
	if err := u.destroyEnviron(ctx, info, retryStrategy); err != nil {
		u.config.Logger.Errorf("destroy environ failed: %v", err)
		return fmt.Errorf("cannot destroy cloud resources: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := u.config.Facade.RemoveModelSecrets(); err != nil {
		u.config.Logger.Errorf("remove model secrets failed: %v", err)
		return errors.Annotate(err, "cannot remove model secrets")
	}

	// Finally, the model is going to be dead, and be removed.
	if err := u.config.Facade.RemoveModel(); err != nil {
		u.config.Logger.Errorf("remove model failed: %v", err)
		return errors.Annotate(err, "cannot remove model")
	}
	return nil
}

func (u *Undertaker) forceDestroy(ctx context.Context, info params.UndertakerModelInfo) error {
	if !info.ForceDestroyed || info.DestroyTimeout == nil {
		return errors.Errorf("invalid force destroy")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if *info.DestroyTimeout == 0 {
		u.config.Logger.Infof("skipping waiting for model to cleanly shutdown since timeout is 0")
	} else if info.Life == life.Dying {
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
		proccessCtx, proccessCancel := context.WithCancel(ctx)
		processTimer := u.config.Clock.AfterFunc(*info.DestroyTimeout, func() {
			proccessCancel()
		})
		defer processTimer.Stop()
		if err := u.processDyingModel(proccessCtx, info); err != nil && !errors.Is(err, context.Canceled) {
			proccessCancel()
			u.config.Logger.Errorf("destroy model failed: %v", err)
			return fmt.Errorf("proccesing model death: %w", err)
		}
		proccessCancel()
	} else {
		u.config.Logger.Debugf("skipping processDyingModel as model is already dead")
	}

	if info.IsSystem {
		// Nothing to do. We don't destroy environ resources or
		// delete model docs for a controller model, because we're
		// running inside that controller and can't safely clean up
		// our own infrastructure. (That'll be the client's job in
		// the end, once we've reported that we've tidied up what we
		// can, by returning nil here, indicating that we've set it
		// to Dead -- implied by processDyingModel succeeding.)
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if *info.DestroyTimeout == 0 {
		u.config.Logger.Infof("skipping tearing down cloud environment since timeout is 0")
	} else {
		destroyCtx, destroyCancel := context.WithCancel(ctx)
		destroyTimer := u.config.Clock.AfterFunc(*info.DestroyTimeout, func() {
			destroyCancel()
		})
		defer destroyTimer.Stop()
		retryStrategy := retry.Exponential{
			Initial:  1 * time.Second,
			Factor:   1.5,
			MaxDelay: 5 * time.Second,
		}
		if err := u.destroyEnviron(destroyCtx, info, retryStrategy); err != nil && !errors.Is(err, context.Canceled) {
			destroyCancel()
			u.config.Logger.Errorf("destroy environ failed: %v", err)
			return fmt.Errorf("tearing down cloud environment: %w", err)
		}
		destroyCancel()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := u.config.Facade.RemoveModelSecrets(); err != nil {
		u.config.Logger.Errorf("remove model secrets failed: %v", err)
	}

	// Finally, the model is going to be dead, and be removed.
	if err := u.config.Facade.RemoveModel(); err != nil {
		u.config.Logger.Errorf("remove model failed: %v", err)
		return errors.Annotate(err, "cannot remove model")
	}
	return nil
}

func (u *Undertaker) environ() (environs.CloudDestroyer, error) {
	modelConfig, err := u.config.Facade.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving model config")
	}

	cloudSpec, err := u.config.Facade.CloudSpec()
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving cloud spec for model %q (%s)", modelConfig.Name(), modelConfig.UUID())
	}

	environ, err := u.config.NewCloudDestroyerFunc(context.TODO(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: modelConfig,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "creating environ for model %q (%s)", modelConfig.Name(), modelConfig.UUID())
	}
	return environ, nil
}

func (u *Undertaker) invokeDestroyEnviron(callCtx environscontext.ProviderCallContext) error {
	environ, err := u.environ()
	if err != nil {
		return err
	}
	return environ.Destroy(callCtx)
}

func (u *Undertaker) destroyEnviron(ctx context.Context, info params.UndertakerModelInfo, retryStrategy retry.Strategy) error {
	u.config.Logger.Debugf("destroying cloud resources for model %v", info.Name)
	// Now the model is known to be hosted and dying, we can tidy up any
	// provider resources it might have used.
	if err := u.setStatus(
		status.Destroying, "tearing down cloud environment",
	); err != nil {
		return errors.Trace(err)
	}

	callCtx := common.NewCloudCallContextFunc(u.config.CredentialAPI)(ctx)
	errChan := make(chan error)
	done := make(chan struct{})
	defer close(done)

	r := retry.Start(retryStrategy, u.config.Clock)
	attempt := 1
	var destroyErr error = errors.ConstError("exhausted retries")
out:
	for r.Next() {
		select {
		case <-ctx.Done():
			destroyErr = ctx.Err()
			break out
		default:
		}
		go func() {
			u.config.Logger.Tracef("environ destroy enter")
			defer u.config.Logger.Tracef("environ destroy leave")
			err := u.invokeDestroyEnviron(callCtx)
			select {
			case errChan <- err:
			case <-done:
				if err != nil {
					u.config.Logger.Errorf("attempt %d to destroy environ failed (will not retry):  %v", attempt, err)
				}
			}
		}()
		select {
		case <-ctx.Done():
			destroyErr = ctx.Err()
			break out
		case destroyErr = <-errChan:
			if destroyErr == nil {
				break out
			}
			u.config.Logger.Errorf("attempt %d to destroy environ failed (will retry):  %v", attempt, destroyErr)
		}
	}
	if destroyErr == nil {
		return nil
	}
	return fmt.Errorf("process destroy environ: %w", destroyErr)
}

func (u *Undertaker) setStatus(modelStatus status.Status, message string) error {
	return u.config.Facade.SetStatus(modelStatus, message, nil)
}

func (u *Undertaker) processDyingModel(ctx context.Context, info params.UndertakerModelInfo) error {
	watch, err := u.config.Facade.WatchModelResources()
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.catacomb.Add(watch); err != nil {
		return errors.Trace(err)
	}
	defer watch.Kill()
	attempt := 1
	for {
		select {
		case <-ctx.Done():
			u.config.Logger.Debugf("processDyingModel timed out")
			return errors.Annotatef(ctx.Err(), "process dying model")
		case <-watch.Changes():
			err := u.config.Facade.ProcessDyingModel()
			if err == nil {
				u.config.Logger.Debugf("processDyingModel done")
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

			u.config.Logger.Debugf("attempt %d to destroy model failed (will retry):  %v", attempt, err)
		}
		attempt++
	}
}
