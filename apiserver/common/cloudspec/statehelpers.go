// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// Pool describes an interface for retrieving State instances from a
// collection.
type Pool interface {
	Get(string) (*state.PooledState, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)

	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// WatchCloud returns a watcher that observes changes to the specified cloud.
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// ModelConfigServiceGetter is a factory for ModelConfigService. It takes a model
// UUID and returns a ModelConfigService for that model.
type ModelConfigServiceGetter func(context.Context, coremodel.UUID) (ModelConfigService, error)

// MakeCloudSpecGetter returns a function which returns a CloudSpec
// for a given model, using the given Pool.
func MakeCloudSpecGetter(pool Pool, cloudService common.CloudService, credentialService CredentialService, modelConfigServiceGetter ModelConfigServiceGetter) func(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, tag names.ModelTag) (environscloudspec.CloudSpec, error) {
		st, err := pool.Get(tag.Id())
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}
		defer st.Release()

		m, err := st.Model()
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}

		modelConfigService, err := modelConfigServiceGetter(ctx, coremodel.UUID(m.UUID()))
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}

		// TODO - CAAS(externalreality): Once cloud methods are migrated
		// to model EnvironConfigGetter will no longer need to contain
		// both state and model but only model.
		// TODO (manadart 2018-02-15): This potentially frees the state from
		// the pool. Release is called, but the state reference survives.
		return stateenvirons.EnvironConfigGetter{
			Model:              m,
			CloudService:       cloudService,
			CredentialService:  credentialService,
			ModelConfigService: modelConfigService,
		}.CloudSpec(ctx)
	}
}

// MakeCloudSpecGetterForModel returns a function which returns a
// CloudSpec for a single model. Attempts to request a CloudSpec for
// any other model other than the one associated with the given
// state.State results in an error.
func MakeCloudSpecGetterForModel(st *state.State, cloudService CloudService, credentialService CredentialService, modelConfigService ModelConfigService) func(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, tag names.ModelTag) (environscloudspec.CloudSpec, error) {
		m, err := st.Model()
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}
		configGetter := stateenvirons.EnvironConfigGetter{
			Model:              m,
			CloudService:       cloudService,
			CredentialService:  credentialService,
			ModelConfigService: modelConfigService,
		}

		if tag.Id() != st.ModelUUID() {
			return environscloudspec.CloudSpec{}, errors.New("cannot get cloud spec for this model")
		}
		return configGetter.CloudSpec(ctx)
	}
}

// MakeCloudSpecWatcherForModel returns a function which returns a
// NotifyWatcher for cloud spec changes for a single model.
// Attempts to request a watcher for any other model other than the
// one associated with the given state.State results in an error.
func MakeCloudSpecWatcherForModel(st *state.State, cloudService CloudService) func(context.Context, names.ModelTag) (watcher.NotifyWatcher, error) {
	return func(ctx context.Context, tag names.ModelTag) (watcher.NotifyWatcher, error) {
		m, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if tag.Id() != st.ModelUUID() {
			return nil, errors.New("cannot get cloud spec for this model")
		}
		return cloudService.WatchCloud(ctx, m.CloudName())
	}
}

// MakeCloudSpecCredentialWatcherForModel returns a function which returns a
// NotifyWatcher for changes to a model's credential reference.
// This watch will detect when model's credential is replaced with another credential.
// Attempts to request a watcher for any other model other than the
// one associated with the given state.State results in an error.
func MakeCloudSpecCredentialWatcherForModel(st *state.State) func(names.ModelTag) (state.NotifyWatcher, error) {
	return func(tag names.ModelTag) (state.NotifyWatcher, error) {
		m, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if tag.Id() != st.ModelUUID() {
			return nil, errors.New("cannot get cloud spec credential for this model")
		}
		return m.WatchModelCredential(), nil
	}
}

// MakeCloudSpecCredentialContentWatcherForModel returns a function which returns a
// NotifyWatcher for credential content changes for a single model.
// Attempts to request a watcher for any other model other than the
// one associated with the given state.State results in an error.
func MakeCloudSpecCredentialContentWatcherForModel(st *state.State, credentialService CredentialService) func(context.Context, names.ModelTag) (watcher.NotifyWatcher, error) {
	return func(ctx context.Context, tag names.ModelTag) (watcher.NotifyWatcher, error) {
		m, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if tag.Id() != st.ModelUUID() {
			return nil, errors.New("cannot get cloud spec credential content for this model")
		}
		credentialTag, exists := m.CloudCredentialTag()
		if !exists {
			return nil, nil
		}
		return credentialService.WatchCredential(ctx, credential.KeyFromTag(credentialTag))
	}
}
