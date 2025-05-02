// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/tomb.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/rpc/params"
)

// CredentialService provides access to perform credentials operations.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)

	// InvalidateCredential marks the cloud credential for the given key as invalid.
	InvalidateCredential(ctx context.Context, key credential.Key, reason string) error
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

// ModelService provides access to the model.
type ModelService interface {
	// WatchModelCloudCredential returns a new NotifyWatcher watching for changes that
	// result in the cloud spec for a model changing. The changes watched for are:
	// - updates to model cloud.
	// - updates to model credential.
	// - changes to the credential set on a model.
	// The following errors can be expected:
	// - [modelerrors.NotFound] when the model is not found.
	WatchModelCloudCredential(ctx context.Context, modelUUID model.UUID) (watcher.NotifyWatcher, error)
}

// ModelInfoService provides access to the model info.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(ctx context.Context) (model.ModelInfo, error)
}

// CredentialValidatorAPIV2 implements the credential validator API V2.
type CredentialValidatorAPIV2 struct {
	*CredentialValidatorAPI
}

// CredentialValidatorAPI implements the credential validator API.
type CredentialValidatorAPI struct {
	logger                       logger.Logger
	cloudService                 CloudService
	credentialService            CredentialService
	modelService                 ModelService
	modelInfoService             ModelInfoService
	modelCredentialWatcherGetter func(ctx context.Context) (watcher.NotifyWatcher, error)
	watcherRegistry              facade.WatcherRegistry
}

func internalNewCredentialValidatorAPI(
	cloudService CloudService, credentialService CredentialService,
	modelService ModelService,
	modelInfoService ModelInfoService,
	modelCredentialWatcherGetter func(ctx context.Context) (watcher.NotifyWatcher, error),
	watcherRegistry facade.WatcherRegistry,
	logger logger.Logger,
) *CredentialValidatorAPI {
	return &CredentialValidatorAPI{
		cloudService:                 cloudService,
		credentialService:            credentialService,
		modelService:                 modelService,
		modelInfoService:             modelInfoService,
		modelCredentialWatcherGetter: modelCredentialWatcherGetter,
		watcherRegistry:              watcherRegistry,
		logger:                       logger,
	}
}

// noopNotifyWatcher provides a notify watcher that fires the
// first event and then sits dormant.
// Used for a compatibility WatchCredential api method.
type noopNotifyWatcher struct {
	tomb tomb.Tomb
	ch   <-chan struct{}
}

func newNoopNotifyWatcher() *noopNotifyWatcher {
	ch := make(chan struct{}, 1)
	// Initial event.
	ch <- struct{}{}
	w := &noopNotifyWatcher{ch: ch}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *noopNotifyWatcher) Changes() <-chan struct{} {
	return w.ch
}

func (w *noopNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *noopNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *noopNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *noopNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

// WatchCredential returns a NotifyWatcher.
// This is only called by 3.6 agents and is a noop since
// [WatchModelCredential] is the only watcher needed for 4.x.
func (api *CredentialValidatorAPIV2) WatchCredential(ctx context.Context, tag params.Entity) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	var err error
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, newNoopNotifyWatcher())
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ModelCredential returns cloud credential information for a  model.
func (api *CredentialValidatorAPI) ModelCredential(ctx context.Context) (params.ModelCredential, error) {
	c, err := api.modelCredential(ctx)
	if err != nil {
		return params.ModelCredential{}, apiservererrors.ServerError(err)
	}

	return params.ModelCredential{
		Model:           c.Model.String(),
		CloudCredential: c.CredentialTag.String(),
		Exists:          c.Exists,
		Valid:           c.Valid,
	}, nil
}

// modelCredential stores model's cloud credential information.
type modelCredential struct {
	// Model is a model tag.
	Model names.ModelTag

	// Exists indicates whether the model has  a cloud credential.
	// On some clouds, that only require "empty" auth, cloud credential
	// is not needed for the models to function properly.
	Exists bool

	// CredentialTag is a cloud credential tag.
	CredentialTag names.CloudCredentialTag

	// Valid indicates that this model's cloud authentication is valid.
	//
	// If this model has a cloud credential setup,
	// then this property indicates that this credential itself is valid.
	//
	// If this model has no cloud credential, then this property indicates
	// whether or not it is valid for this model to have no credential.
	// There are some clouds that do not require auth and, hence,
	// models on these clouds do not require credentials.
	//
	// If a model is on the cloud that does require credential and
	// the model's credential is not set, this property will be set to 'false'.
	Valid bool
}

func (api *CredentialValidatorAPI) modelCredential(ctx context.Context) (*modelCredential, error) {
	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	exists := modelInfo.CredentialName != ""
	modelTag := names.NewModelTag(modelInfo.UUID.String())

	result := &modelCredential{Model: modelTag, Exists: exists}
	if !exists {
		// A model credential is not set, we must check if the model
		// is on the cloud that requires a credential.
		supportsEmptyAuth, err := api.cloudSupportsNoAuth(ctx, modelInfo.Cloud)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Valid = supportsEmptyAuth
		if !supportsEmptyAuth {
			// TODO (anastasiamac 2018-11-12) Figure out how to notify the users here - maybe set a model status?...
			api.logger.Warningf(ctx, "model credential is not set for the model but the cloud requires it")
		}
		return result, nil
	}

	modelCredentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: modelInfo.CredentialOwner,
		Name:  modelInfo.CredentialName,
	}

	modelCredentialTag, err := modelCredentialKey.Tag()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.CredentialTag = modelCredentialTag

	credential, err := api.credentialService.CloudCredential(ctx, modelCredentialKey)
	if err != nil {
		if !errors.Is(err, credentialerrors.CredentialNotFound) {
			return nil, errors.Trace(err)
		}
		// In this situation, a model refers to a credential that does not exist in credentials collection.
		// TODO (anastasiamac 2018-11-12) Figure out how to notify the users here - maybe set a model status?...
		api.logger.Warningf(ctx, "cloud credential reference is set for the model but the credential content is no longer on the controller")
		result.Valid = false
		return result, nil
	}
	result.Valid = !credential.Invalid

	return result, nil
}

func (api *CredentialValidatorAPI) cloudSupportsNoAuth(ctx context.Context, cloudName string) (bool, error) {
	cl, err := api.cloudService.Cloud(ctx, cloudName)
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, authType := range cl.AuthTypes {
		if authType == cloud.EmptyAuthType {
			return true, nil
		}
	}
	return false, nil
}

// WatchModelCredential returns a NotifyWatcher that watches what cloud credential a model uses.
func (api *CredentialValidatorAPI) WatchModelCredential(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watcher, err := api.modelCredentialWatcherGetter(ctx)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}

	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watcher)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// InvalidateModelCredential marks the cloud credential for this model as invalid.
// This is only used by 3.6 agents and can be dropped in 4.x when we no
// longer need to support migrating 3.6 models.
func (api *CredentialValidatorAPIV2) InvalidateModelCredential(ctx context.Context, args params.InvalidateCredentialArg) (params.ErrorResult, error) {
	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.ErrorResult{}, errors.Trace(err)
	}
	modelCredentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: modelInfo.CredentialOwner,
		Name:  modelInfo.CredentialName,
	}
	err = api.credentialService.InvalidateCredential(ctx, modelCredentialKey, args.Reason)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}
