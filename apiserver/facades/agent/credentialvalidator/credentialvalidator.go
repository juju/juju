// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// CredentialValidatorV2 defines the methods on version 2 facade for the
// credentialvalidator API endpoint.
type CredentialValidatorV2 interface {
	InvalidateModelCredential(context.Context, params.InvalidateCredentialArg) (params.ErrorResult, error)
	ModelCredential(context.Context) (params.ModelCredential, error)
	WatchCredential(context.Context, params.Entity) (params.NotifyWatchResult, error)
	WatchModelCredential(context.Context) (params.NotifyWatchResult, error)
}

type CredentialService interface {
	common.CredentialService
	InvalidateCredential(ctx context.Context, key credential.Key, reason string) error
}

type CredentialValidatorAPI struct {
	*credentialcommon.CredentialManagerAPI

	logger            loggo.Logger
	backend           StateAccessor
	cloudService      common.CloudService
	credentialService CredentialService
	resources         facade.Resources
}

var (
	_ CredentialValidatorV2 = (*CredentialValidatorAPI)(nil)
)

func internalNewCredentialValidatorAPI(
	backend StateAccessor, cloudService common.CloudService, credentialService CredentialService, resources facade.Resources,
	authorizer facade.Authorizer, logger loggo.Logger,
) (*CredentialValidatorAPI, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, apiservererrors.ErrPerm
	}

	return &CredentialValidatorAPI{
		CredentialManagerAPI: credentialcommon.NewCredentialManagerAPI(backend, credentialService),
		resources:            resources,
		backend:              backend,
		cloudService:         cloudService,
		credentialService:    credentialService,
		logger:               logger,
	}, nil
}

// WatchCredential returns a NotifyWatcher that observes
// changes to a given cloud credential.
func (api *CredentialValidatorAPI) WatchCredential(ctx context.Context, tag params.Entity) (params.NotifyWatchResult, error) {
	fail := func(failure error) (params.NotifyWatchResult, error) {
		return params.NotifyWatchResult{}, apiservererrors.ServerError(failure)
	}

	credentialTag, err := names.ParseCloudCredentialTag(tag.Tag)
	if err != nil {
		return fail(err)
	}
	// Is credential used by the model that has created this backend?
	modelCredentialTag, exists, err := api.backend.CloudCredentialTag()
	if err != nil {
		return fail(err)
	}
	if !exists || credentialTag != modelCredentialTag {
		return fail(apiservererrors.ErrPerm)
	}

	result := params.NotifyWatchResult{}
	watch, err := api.credentialService.WatchCredential(ctx, credential.KeyFromTag(credentialTag))
	if errors.Is(err, credentialerrors.NotFound) {
		err = fmt.Errorf("credential %q %w", credentialTag, errors.NotFound)
	}
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	// Consume the initial event. Technically, API calls to Watch
	// 'transmit' the initial event in the Watch response. But
	// NotifyWatchers have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = api.resources.Register(watch)
	} else {
		watch.Kill()
		result.Error = apiservererrors.ServerError(watch.Wait())
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
		CloudCredential: c.Credential.String(),
		Exists:          c.Exists,
		Valid:           c.Valid,
	}, nil
}

func (api *CredentialValidatorAPI) modelCredential(ctx context.Context) (*ModelCredential, error) {
	m, err := api.backend.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelCredentialTag, exists, err := api.backend.CloudCredentialTag()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &ModelCredential{Model: m.ModelTag(), Exists: exists}
	if !exists {
		// A model credential is not set, we must check if the model
		// is on the cloud that requires a credential.
		supportsEmptyAuth, err := api.cloudSupportsNoAuth(ctx, m.CloudName())
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Valid = supportsEmptyAuth
		if !supportsEmptyAuth {
			// TODO (anastasiamac 2018-11-12) Figure out how to notify the users here - maybe set a model status?...
			api.logger.Warningf("model credential is not set for the model but the cloud requires it")
		}
		return result, nil
	}

	result.Credential = modelCredentialTag
	credential, err := api.credentialService.CloudCredential(ctx, credential.KeyFromTag(modelCredentialTag))
	if err != nil {
		if !errors.Is(err, credentialerrors.CredentialNotFound) {
			return nil, errors.Trace(err)
		}
		// In this situation, a model refers to a credential that does not exist in credentials collection.
		// TODO (anastasiamac 2018-11-12) Figure out how to notify the users here - maybe set a model status?...
		api.logger.Warningf("cloud credential reference is set for the model but the credential content is no longer on the controller")
		result.Valid = false
		return result, nil
	}
	result.Valid = !credential.Invalid
	return result, nil
}

func (api *CredentialValidatorAPI) cloudSupportsNoAuth(ctx context.Context, cloudName string) (bool, error) {
	cloud, err := api.cloudService.Cloud(ctx, cloudName)
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, authType := range cloud.AuthTypes {
		if authType == jujucloud.EmptyAuthType {
			return true, nil
		}
	}
	return false, nil
}

// WatchModelCredential returns a NotifyWatcher that watches what cloud credential a model uses.
func (api *CredentialValidatorAPI) WatchModelCredential(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	m, err := api.backend.Model()
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	watch := m.WatchModelCredential()

	// Consume the initial event. Technically, API calls to Watch
	// 'transmit' the initial event in the Watch response. But
	// NotifyWatchers have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = api.resources.Register(watch)
	} else {
		err = watcher.EnsureErr(watch)
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
