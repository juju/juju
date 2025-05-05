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
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// CredentialService exposed service methods for interacting with any model's
// credentials. This is used to define the exact interface that is required by
// [credentialServiceShim].
type CredentialService interface {
	// GetModelCredentialStatus returns the credential key that is in use by the
	// model and also a bool indicating of the credential is considered valid.
	// The following errors can be expected:
	// - [credentialerrors.ModelCredentialNotSet] when the model does not have
	// any credential set.
	// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does
	// not exist.
	GetModelCredentialStatus(context.Context, coremodel.UUID) (corecredential.Key, bool, error)

	// InvalidateModelCredential marks the cloud credential that is being used
	// by the model uuid as invalid. This will affect all models that are using
	// the credential.
	// The following errors can be expected:
	// - [github.com/juju/juju/core/errors.NotValid] when the modelUUID is not
	// valid.
	// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does
	// not exist.
	InvalidateModelCredential(context.Context, coremodel.UUID, string) error
}

// credentialServiceShim is a shim that implements the [ModelCredentialService]
// on top of an existing [CredentialService]. This exists so that the caller
// is scoped to that of a single model and cannot move sideways to other models
// in the controller.
type credentialServiceShim struct {
	ModelUUID coremodel.UUID
	Service   CredentialService
}

// ModelCredentialService exposes State methods needed by credential manager.
// ModelCredentialService expo
type ModelCredentialService interface {
	// GetModelCredentialStatus returns the credential key that is in use by the
	// model and also a bool indicating of the credential is considered valid.
	// The following errors can be expected:
	// - [credentialerrors.ModelCredentialNotSet] when the model does not have
	// any credential set.
	// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does
	// not exist.
	GetModelCredentialStatus(context.Context) (corecredential.Key, bool, error)

	// InvalidateModelCredential marks the cloud credential that is in use for
	// by the model as invalid. This will affect all models that are using the
	// credential.
	// The following errors can be expected:
	// - [github.com/juju/juju/core/errors.NotValid] when the modelUUID is not
	// valid.
	// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does
	// not exist.
	InvalidateModelCredential(context.Context, string) error
}

// CredentialValidatorAPIV2 implements the credential validator API V2.
type CredentialValidatorAPIV2 struct {
	*CredentialValidatorAPI
}

// CredentialValidatorAPI implements the credential validator API.
type CredentialValidatorAPI struct {
	credentialService            ModelCredentialService
	modelTag                     names.ModelTag
	modelCredentialWatcherGetter func(ctx context.Context) (watcher.NotifyWatcher, error)
	watcherRegistry              facade.WatcherRegistry
}

// GetModelCredentialStatus returns the credential key that is in use by the
// model and also a bool indicating of the credential is considered valid.
// The following errors can be expected:
// - [credentialerrors.ModelCredentialNotSet] when the model does not have
// any credential set.
// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does
// not exist.
//
// Implements [ModelCredentialService].
func (s *credentialServiceShim) GetModelCredentialStatus(
	ctx context.Context,
) (corecredential.Key, bool, error) {
	return s.Service.GetModelCredentialStatus(ctx, s.ModelUUID)
}

// InvalidateModelCredential marks the cloud credential that is in use for
// by the model as invalid. This will affect all models that are using the
// credential.
// The following errors can be expected:
// - [github.com/juju/juju/core/errors.NotValid] when the modelUUID is not
// valid.
// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does
// not exist.
//
// Implements [ModelCredentialService].
func (s *credentialServiceShim) InvalidateModelCredential(
	ctx context.Context,
	reason string,
) error {
	return s.Service.InvalidateModelCredential(ctx, s.ModelUUID, reason)
}

func NewCredentialValidatorAPI(
	modelUUID coremodel.UUID,
	credentialService ModelCredentialService,
	modelCredentialWatcherGetter func(ctx context.Context) (watcher.NotifyWatcher, error),
	watcherRegistry facade.WatcherRegistry,
) *CredentialValidatorAPI {
	return &CredentialValidatorAPI{
		modelTag:                     names.NewModelTag(modelUUID.String()),
		credentialService:            credentialService,
		modelCredentialWatcherGetter: modelCredentialWatcherGetter,
		watcherRegistry:              watcherRegistry,
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
	exists := true
	credKey, valid, err := api.credentialService.GetModelCredentialStatus(ctx)
	switch {
	case errors.Is(err, credentialerrors.ModelCredentialNotSet):
		valid = true
		exists = false
	case errors.Is(err, modelerrors.NotFound):
		return params.ModelCredential{}, internalerrors.New(
			"model does not exist",
		).Add(coreerrors.NotFound)
	case err != nil:
		return params.ModelCredential{}, internalerrors.Errorf(
			"getting model credential status information: %w", err,
		)
	}

	credTag, err := credKey.Tag()
	if err != nil {
		return params.ModelCredential{}, internalerrors.Errorf(
			"parsing credential key %q to tag: %w", credKey, err,
		)
	}

	return params.ModelCredential{
		Model:           api.modelTag.String(),
		CloudCredential: credTag.String(),
		Exists:          exists,
		Valid:           valid,
	}, nil
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
	err := api.credentialService.InvalidateModelCredential(ctx, args.Reason)
	switch {
	case errors.Is(err, modelerrors.NotFound):
		return params.ErrorResult{
			Error: apiservererrors.ParamsErrorf(
				params.CodeNotFound,
				"model does not exist",
			),
		}, nil
	case err != nil:
		return params.ErrorResult{}, err
	}

	return params.ErrorResult{}, nil
}
