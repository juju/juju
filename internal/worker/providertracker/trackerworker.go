// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"reflect"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher/eventsource"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// TrackerConfig describes the dependencies of a Worker.
type TrackerConfig struct {
	ModelService       ModelService
	CloudService       CloudService
	ConfigService      ConfigService
	CredentialService  CredentialService
	GetProviderForType func(coremodel.ModelType) (GetProviderFunc, error)
	Logger             logger.Logger
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config TrackerConfig) Validate() error {
	if config.CloudService == nil {
		return errors.NotValidf("nil CloudService")
	}
	if config.ConfigService == nil {
		return errors.NotValidf("nil ConfigService")
	}
	if config.CredentialService == nil {
		return errors.NotValidf("nil CredentialService")
	}
	if config.GetProviderForType == nil {
		return errors.NotValidf("nil GetProviderForType")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// trackerWorker loads an environment, makes it available to clients, and updates
// the environment in response to config changes until it is killed.
type trackerWorker struct {
	catacomb       catacomb.Catacomb
	internalStates chan string

	config           TrackerConfig
	model            coremodel.ModelInfo
	provider         Provider
	currentCloudSpec environscloudspec.CloudSpec

	providerGetter trackerProviderGetter

	// providerReady is closed once the provider has been built inside loop().
	// The constructor blocks on this channel so that Provider() is always
	// usable when the worker is returned.
	providerReady  chan struct{}
	closeReadyOnce sync.Once
}

// NewTrackerWorker loads a provider from the observer and returns a new Worker,
// or an error if anything goes wrong. If a tracker is returned, its Environ()
// method is immediately usable.
func NewTrackerWorker(ctx context.Context, config TrackerConfig) (worker.Worker, error) {
	return newTrackerWorker(ctx, config, nil)
}

type invalidateCredentialFunc func(context.Context, environs.CredentialInvalidReason) error

func (f invalidateCredentialFunc) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return f(ctx, reason)
}

// signalProviderReady closes the providerReady channel exactly once.
// Called from loop() when the provider has been built (or on error).
func (t *trackerWorker) signalProviderReady() {
	t.closeReadyOnce.Do(func() {
		close(t.providerReady)
	})
}

func newTrackerWorker(ctx context.Context, config TrackerConfig, internalStates chan string) (*trackerWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Read model info early only to obtain the UUID needed for the
	// credential watcher. The full provider is built inside loop()
	// after all watcher subscriptions are active.
	model, err := config.ModelService.Model(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &trackerWorker{
		internalStates: internalStates,
		config:         config,
		model:          model,
		providerReady:  make(chan struct{}),
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "provider-tracker",
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Block until the provider has been built inside loop(). This
	// guarantees that Provider() is usable when the worker is returned.
	select {
	case <-t.providerReady:
	case <-t.catacomb.Dying():
		return nil, t.catacomb.ErrDying()
	}

	return t, nil
}

// Provider returns the encapsulated Environ. It will continue to be updated in
// the background for as long as the Worker continues to run.
func (t *trackerWorker) Provider() Provider {
	return t.provider
}

// ModelType returns the type of the model.
func (t *trackerWorker) ModelType() coremodel.ModelType {
	return t.model.Type
}

// Kill is part of the worker.Worker interface.
func (t *trackerWorker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *trackerWorker) Wait() error {
	return t.catacomb.Wait()
}

func (t *trackerWorker) Report(ctx context.Context) map[string]any {
	report := map[string]any{
		"model": t.model.UUID,
		"type":  t.model.Type,
		"cloud-spec": map[string]any{
			"type":                t.currentCloudSpec.Type,
			"name":                t.currentCloudSpec.Name,
			"region":              t.currentCloudSpec.Region,
			"endpoint":            t.currentCloudSpec.Endpoint,
			"identity-endpoint":   t.currentCloudSpec.IdentityEndpoint,
			"storage-endpoint":    t.currentCloudSpec.StorageEndpoint,
			"skip-tls-verify":     t.currentCloudSpec.SkipTLSVerify,
			"is-controller-cloud": t.currentCloudSpec.IsControllerCloud,
		},
	}

	if t.provider != nil && t.provider.Config() != nil {
		report["provider"] = t.provider.Config().Name()
	}

	return report
}

func (t *trackerWorker) loop() (err error) {
	ctx, cancel := t.scopedContext()
	defer cancel()

	// Subscribe to watchers first. The initial event from each watcher
	// serves as the readiness barrier — the subscription is active after
	// we receive and consume it. Any state change after that point will
	// be caught by the watcher.

	modelConfigWatcher, err := t.config.ConfigService.Watch(ctx)
	if err != nil {
		t.signalProviderReady()
		return errors.Annotate(err, "watching model config")
	}
	if err := t.addStringsWatcher(ctx, modelConfigWatcher); err != nil {
		t.signalProviderReady()
		return errors.Trace(err)
	}

	modelWatcher, err := t.config.ModelService.WatchModel(ctx)
	if errors.Is(err, modelerrors.NotFound) {
		t.signalProviderReady()
		return nil
	} else if err != nil {
		t.signalProviderReady()
		return errors.Annotate(err, "watching model")
	}
	if err := t.addNotifyWatcher(ctx, modelWatcher); err != nil {
		t.signalProviderReady()
		return errors.Trace(err)
	}

	// Now that both config and model watchers are subscribed (readiness
	// barriers passed), build the provider from fresh state. Any model
	// config or credential change after the subscriptions above will be
	// caught by the main loop.

	getter := trackerProviderGetter{
		model:             t.model,
		cloudService:      t.config.CloudService,
		configService:     t.config.ConfigService,
		credentialService: t.config.CredentialService,
	}
	newProviderType, err := t.config.GetProviderForType(t.model.Type)
	if err != nil {
		t.signalProviderReady()
		return errors.Trace(err)
	}

	var invalidateCredential invalidateCredentialFunc = func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		return t.config.CredentialService.InvalidateCredential(ctx, credential.Key{
			Cloud: t.model.Cloud,
			Owner: t.model.CredentialOwner,
			Name:  t.model.CredentialName,
		}, string(reason))
	}
	provider, spec, err := newProviderType(ctx, getter, invalidateCredential)
	if err != nil {
		t.signalProviderReady()
		return errors.Trace(err)
	}

	t.provider = provider
	t.currentCloudSpec = spec
	t.providerGetter = getter

	cfg := t.provider.Config()
	defer errors.DeferredAnnotatef(&err, "model %q (%s)", cfg.Name(), cfg.UUID())

	// If the provider supports dynamic cloud spec updates, subscribe
	// to the credential watcher before signaling readiness. This
	// ensures all watcher subscriptions are active before the
	// constructor returns, so no credential change can be missed
	// during the startup window. After subscribing, sync the cloud
	// spec to catch any credential change that occurred between
	// building the provider and subscribing the watcher.
	var cloudSpecChanges <-chan struct{}

	cloudSpecSetter, ok := any(t.provider).(environs.CloudSpecSetter)
	if ok {
		cloudSpecChanges, err = t.watchCloudSpecChanges(ctx)
		if err != nil {
			t.signalProviderReady()
			return errors.Annotate(err, "watching credential")
		}
		if err := t.updateCloudSpec(ctx, cloudSpecSetter); err != nil {
			t.signalProviderReady()
			return errors.Annotate(err, "syncing cloud spec")
		}
	} else {
		t.config.Logger.Warningf(ctx, "cloud type %v doesn't support dynamic changing of cloud spec", cfg.Type())
	}

	// Provider is ready — unblock the constructor.
	t.signalProviderReady()

	// Report the initial started state.
	t.reportInternalState(stateStarted)

	logger := t.config.Logger

	for {
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()

		case _, ok := <-modelConfigWatcher.Changes():
			if !ok {
				return errors.New("model config watch closed")
			}
			logger.Debugf(ctx, "reloading model config")

			modelConfig, err := t.config.ConfigService.ModelConfig(ctx)
			if errors.Is(err, modelerrors.NotFound) {
				logger.Infof(ctx, "model %q (%s) has been removed, stopping tracker worker", t.model.Name, t.model.UUID)
				return nil
			} else if err != nil {
				return errors.Annotate(err, "reading model config")
			}
			if err = t.provider.SetConfig(ctx, modelConfig); err != nil {
				return errors.Annotate(err, "updating provider config")
			}

		case _, ok := <-cloudSpecChanges:
			if !ok {
				return errors.New("credential watch closed")
			}
			logger.Debugf(ctx, "reloading credential")

			if err := t.updateCloudSpec(ctx, cloudSpecSetter); err != nil {
				return errors.Annotate(err, "updating cloud spec")
			}

		case <-modelWatcher.Changes():
			model, err := t.config.ModelService.Model(ctx)
			if errors.Is(err, modelerrors.NotFound) {
				// The model has been removed, we can stop the worker.
				logger.Infof(ctx, "model %q (%s) has been removed, stopping tracker worker", t.model.Name, t.model.UUID)
				return nil
			} else if err != nil {
				return errors.Annotate(err, "reading model")
			}
			if corelife.IsDead(model.Life) {
				// The model is dead, we can stop the worker.
				logger.Infof(ctx, "model %q (%s) is dead, stopping tracker worker", model.Name, model.UUID)
				return nil
			}
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (t *trackerWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return t.catacomb.Context(ctx), cancel
}

func (t *trackerWorker) reportInternalState(state string) {
	select {
	case <-t.catacomb.Dying():
	case t.internalStates <- state:
	default:
	}
}

func (t *trackerWorker) watchCloudSpecChanges(ctx context.Context) (<-chan struct{}, error) {
	credentialWatcher, err := t.config.ModelService.WatchModelCloudCredential(ctx, t.model.UUID)
	if err != nil {
		return nil, errors.Annotate(err, "watching model credential")
	}
	if err := t.addNotifyWatcher(ctx, credentialWatcher); err != nil {
		return nil, errors.Trace(err)
	}
	return credentialWatcher.Changes(), nil
}

func (t *trackerWorker) updateCloudSpec(ctx context.Context, cloudSetter environs.CloudSpecSetter) error {
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

func (t *trackerWorker) addNotifyWatcher(ctx context.Context, watcher eventsource.Watcher[struct{}]) error {
	if err := t.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent(ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (t *trackerWorker) addStringsWatcher(ctx context.Context, watcher eventsource.Watcher[[]string]) error {
	if err := t.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent(ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type trackerProviderGetter struct {
	model             coremodel.ModelInfo
	cloudService      CloudService
	configService     ConfigService
	credentialService CredentialService
}

// ControllerUUID returns the controller UUID.
func (g trackerProviderGetter) ControllerUUID(context.Context) (string, error) {
	return g.model.ControllerUUID.String(), nil
}

// ModelConfig returns the model config.
func (g trackerProviderGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.configService.ModelConfig(ctx)
}

// CloudSpec returns the cloud spec for the model.
func (g trackerProviderGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
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

func modelCredentials(ctx context.Context, credentialService CredentialService, model coremodel.ModelInfo) (*cloud.Credential, error) {
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
