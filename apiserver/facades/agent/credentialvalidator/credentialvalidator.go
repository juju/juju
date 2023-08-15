// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"

	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
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

// CredentialValidatorV1 defines the methods on version 1 facade
// for the credentialvalidator API endpoint.
type CredentialValidatorV1 interface {
	InvalidateModelCredential(context.Context, params.InvalidateCredentialArg) (params.ErrorResult, error)
	ModelCredential(context.Context) (params.ModelCredential, error)
	WatchCredential(context.Context, params.Entity) (params.NotifyWatchResult, error)
}

type CredentialValidatorAPI struct {
	*credentialcommon.CredentialManagerAPI

	backend   Backend
	resources facade.Resources
}

var (
	_ CredentialValidatorV2 = (*CredentialValidatorAPI)(nil)
)

func internalNewCredentialValidatorAPI(backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*CredentialValidatorAPI, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, apiservererrors.ErrPerm
	}

	return &CredentialValidatorAPI{
		CredentialManagerAPI: credentialcommon.NewCredentialManagerAPI(backend),
		resources:            resources,
		backend:              backend,
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
	isUsed, err := api.backend.ModelUsesCredential(credentialTag)
	if err != nil {
		return fail(err)
	}
	if !isUsed {
		return fail(apiservererrors.ErrPerm)
	}

	result := params.NotifyWatchResult{}
	watch := api.backend.WatchCredential(credentialTag)
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

// ModelCredential returns cloud credential information for a  model.
func (api *CredentialValidatorAPI) ModelCredential(ctx context.Context) (params.ModelCredential, error) {
	c, err := api.backend.ModelCredential()
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

// WatchModelCredential returns a NotifyWatcher that watches what cloud credential a model uses.
func (api *CredentialValidatorAPI) WatchModelCredential(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch, err := api.backend.WatchModelCredential()
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}

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
