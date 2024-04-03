// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// Config describes the dependencies of a Worker.
//
// It's arguable that it should be called WorkerConfig, because of the heavy
// use of model config in this package.
type Config[T Provider] struct {
	ModelService      ModelService
	CloudService      CloudService
	ConfigService     ConfigService
	CredentialService CredentialService
	GetProvider       GetProviderFunc[T]
	Logger            Logger
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config Config[T]) Validate() error {
	if config.CloudService == nil {
		return errors.NotValidf("nil CloudService")
	}
	if config.ConfigService == nil {
		return errors.NotValidf("nil ConfigService")
	}
	if config.CredentialService == nil {
		return errors.NotValidf("nil CredentialService")
	}
	if config.GetProvider == nil {
		return errors.NotValidf("nil GetProvider")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// trackerWorker loads an environment, makes it available to clients, and updates
// the environment in response to config changes until it is killed.
type trackerWorker[T Provider] struct {
	catacomb       catacomb.Catacomb
	internalStates chan string

	config           Config[T]
	model            coremodel.ReadOnlyModel
	provider         T
	currentCloudSpec environscloudspec.CloudSpec

	providerGetter providerGetter
}

// NewWorker loads a provider from the observer and returns a new Worker,
// or an error if anything goes wrong. If a tracker is returned, its Environ()
// method is immediately usable.
func NewWorker[T Provider](ctx context.Context, config Config[T]) (worker.Worker, error) {
	return newWorker(ctx, config, nil)
}

func newWorker[T Provider](ctx context.Context, config Config[T], internalStates chan string) (*trackerWorker[T], error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	model, err := config.ModelService.Model(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	getter := providerGetter{
		model:             model,
		cloudService:      config.CloudService,
		configService:     config.ConfigService,
		credentialService: config.CredentialService,
	}
	// Given the model, we can now get the provider.
	provider, spec, err := config.GetProvider(ctx, getter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &trackerWorker[T]{
		internalStates:   internalStates,
		config:           config,
		model:            model,
		provider:         provider,
		currentCloudSpec: spec,
		providerGetter:   getter,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return t, nil
}

// Environ returns the encapsulated Environ. It will continue to be updated in
// the background for as long as the Worker continues to run.
func (t *trackerWorker[T]) Provider() T {
	return t.provider
}

// Kill is part of the worker.Worker interface.
func (t *trackerWorker[T]) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *trackerWorker[T]) Wait() error {
	return t.catacomb.Wait()
}

func (t *trackerWorker[T]) loop() (err error) {
	cfg := t.provider.Config()
	defer errors.DeferredAnnotatef(&err, "model %q (%s)", cfg.Name(), cfg.UUID())

	ctx, cancel := t.scopedContext()
	defer cancel()

	modelConfigWatcher, err := t.config.ConfigService.Watch()
	if err != nil {
		return errors.Annotate(err, "watching model config")
	}
	if err := t.addStringsWatcher(ctx, modelConfigWatcher); err != nil {
		return errors.Trace(err)
	}

	// Empty channels block forever, so we can just return them here, then
	// the caller can ignore them.
	var (
		cloudChanges      <-chan struct{}
		credentialChanges <-chan struct{}
	)

	// Not every provider supports updating the cloud spec, we only want
	// to get the cloud and credential watchers if the provider supports it.
	cloudSpecSetter, ok := any(t.provider).(environs.CloudSpecSetter)
	if ok {
		cloudChanges, err = t.watchCloudChanges(ctx)
		if err != nil {
			return errors.Annotate(err, "watching cloud")
		}
		credentialChanges, err = t.watchCredentialChanges(ctx)
		if err != nil {
			return errors.Annotate(err, "watching credential")
		}
	} else {
		t.config.Logger.Warningf("cloud type %v doesn't support dynamic changing of cloud spec", cfg.Type())
	}

	logger := t.config.Logger

	// Report the initial started state.
	t.reportInternalState(stateStarted)

	for {
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()

		case _, ok := <-modelConfigWatcher.Changes():
			if !ok {
				return errors.New("model config watch closed")
			}
			logger.Debugf("reloading model config")

			modelConfig, err := t.config.ConfigService.ModelConfig(ctx)
			if err != nil {
				return errors.Annotate(err, "reading model config")
			}
			if err = t.provider.SetConfig(modelConfig); err != nil {
				return errors.Annotate(err, "updating provider config")
			}

		case _, ok := <-cloudChanges:
			if !ok {
				return errors.New("cloud watch closed")
			}
			logger.Debugf("reloading cloud")

			if err := t.updateCloudSpec(ctx, cloudSpecSetter); err != nil {
				return errors.Annotate(err, "updating cloud spec")
			}

		case _, ok := <-credentialChanges:
			if !ok {
				return errors.New("credential watch closed")
			}
			logger.Debugf("reloading credential")

			if err := t.updateCloudSpec(ctx, cloudSpecSetter); err != nil {
				return errors.Annotate(err, "updating cloud spec")
			}
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (t *trackerWorker[T]) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return t.catacomb.Context(ctx), cancel
}

func (t *trackerWorker[T]) reportInternalState(state string) {
	select {
	case <-t.catacomb.Dying():
	case t.internalStates <- state:
	default:
	}
}

func (t *trackerWorker[T]) watchCloudChanges(ctx context.Context) (<-chan struct{}, error) {
	cloudWatcher, err := t.config.CloudService.WatchCloud(ctx, t.model.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "watching cloud")
	}
	if err := t.addNotifyWatcher(ctx, cloudWatcher); err != nil {
		return nil, errors.Trace(err)
	}
	return cloudWatcher.Changes(), nil
}

func (t *trackerWorker[T]) watchCredentialChanges(ctx context.Context) (<-chan struct{}, error) {
	credentialName := t.model.CredentialName
	if credentialName == "" {
		return nil, nil
	}

	credentialWatcher, err := t.config.CredentialService.WatchCredential(ctx, credential.Key{
		Cloud: t.model.Cloud,
		Owner: t.model.CredentialOwner,
		Name:  t.model.CredentialName,
	})
	if err != nil {
		return nil, errors.Annotate(err, "watching credential")
	}
	if err := t.addNotifyWatcher(ctx, credentialWatcher); err != nil {
		return nil, errors.Trace(err)
	}
	return credentialWatcher.Changes(), nil
}

func (t *trackerWorker[T]) updateCloudSpec(ctx context.Context, cloudSetter environs.CloudSpecSetter) error {
	spec, err := t.providerGetter.CloudSpec(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting cloud spec")
	}

	// If the spec hasn't changed, we don't need to do anything.
	if reflect.DeepEqual(t.currentCloudSpec, spec) {
		return nil
	}

	// Now update the cloud spec on the provider.
	if err := cloudSetter.SetCloudSpec(ctx, spec); err != nil {
		return errors.Annotate(err, "updating cloud spec")
	}

	t.currentCloudSpec = spec
	return nil
}

func (t *trackerWorker[T]) addNotifyWatcher(ctx context.Context, watcher eventsource.Watcher[struct{}]) error {
	if err := t.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[struct{}](ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (t *trackerWorker[T]) addStringsWatcher(ctx context.Context, watcher eventsource.Watcher[[]string]) error {
	if err := t.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[[]string](ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type providerGetter struct {
	model             coremodel.ReadOnlyModel
	cloudService      CloudService
	configService     ConfigService
	credentialService CredentialService
}

// ControllerUUID returns the controller UUID.
func (g providerGetter) ControllerUUID() coremodel.UUID {
	return g.model.ControllerUUID
}

// ModelUUID returns the model UUID.
func (g providerGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.configService.ModelConfig(ctx)
}

// CloudSpec returns the cloud spec for the model.
func (g providerGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	modelCredentials, err := modelCredentials(ctx, g.credentialService, g.model)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	modelCloud, err := g.cloudService.Cloud(ctx, g.model.Cloud)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	return environscloudspec.MakeCloudSpec(*modelCloud, g.model.CloudRegion, modelCredentials)
}

func modelCredentials(ctx context.Context, credentialService CredentialService, model coremodel.ReadOnlyModel) (*cloud.Credential, error) {
	if model.CredentialName == "" {
		return nil, nil
	}

	credentialValue, err := credentialService.CloudCredential(ctx, credential.Key{
		Cloud: model.Cloud,
		Owner: model.CredentialOwner,
		Name:  model.CredentialName,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudCredential := cloud.NewNamedCredential(
		credentialValue.Label,
		credentialValue.AuthType(),
		credentialValue.Attributes(),
		credentialValue.Revoked,
	)
	return &cloudCredential, nil

}
