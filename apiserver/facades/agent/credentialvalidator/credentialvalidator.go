// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.api.credentialvalidator")

// CredentialValidatorV2 defines the methods on version 2 facade for the
// credentialvalidator API endpoint.
type CredentialValidatorV2 interface {
	InvalidateModelCredential(params.InvalidateCredentialArg) (params.ErrorResult, error)
	ModelCredential() (params.ModelCredential, error)
	WatchCredential(params.Entity) (params.NotifyWatchResult, error)
	WatchModelCredential() (params.NotifyWatchResult, error)
}

// CredentialValidatorV1 defines the methods on version 1 facade
// for the credentialvalidator API endpoint.
type CredentialValidatorV1 interface {
	InvalidateModelCredential(params.InvalidateCredentialArg) (params.ErrorResult, error)
	ModelCredential() (params.ModelCredential, error)
	WatchCredential(params.Entity) (params.NotifyWatchResult, error)
}

type CredentialValidatorAPI struct {
	*credentialcommon.CredentialManagerAPI

	backend   Backend
	resources facade.Resources
}

type CredentialValidatorAPIV1 struct {
	*CredentialValidatorAPI
}

var (
	_ CredentialValidatorV2 = (*CredentialValidatorAPI)(nil)
	_ CredentialValidatorV1 = (*CredentialValidatorAPIV1)(nil)
)

// NewCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func NewCredentialValidatorAPI(ctx facade.Context) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(NewBackend(NewStateShim(ctx.State())), ctx.Resources(), ctx.Auth())
}

// NewCredentialValidatorAPIv1 creates a new CredentialValidator API endpoint on server-side.
func NewCredentialValidatorAPIv1(ctx facade.Context) (*CredentialValidatorAPIV1, error) {
	v2, err := NewCredentialValidatorAPI(ctx)
	if err != nil {
		return nil, err
	}
	return &CredentialValidatorAPIV1{v2}, nil
}

func internalNewCredentialValidatorAPI(backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*CredentialValidatorAPI, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, common.ErrPerm
	}

	return &CredentialValidatorAPI{
		CredentialManagerAPI: credentialcommon.NewCredentialManagerAPI(backend),
		resources:            resources,
		backend:              backend,
	}, nil
}

// WatchCredential returns a NotifyWatcher that observes
// changes to a given cloud credential.
func (api *CredentialValidatorAPI) WatchCredential(tag params.Entity) (params.NotifyWatchResult, error) {
	fail := func(failure error) (params.NotifyWatchResult, error) {
		return params.NotifyWatchResult{}, common.ServerError(failure)
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
		return fail(common.ErrPerm)
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
		result.Error = common.ServerError(err)
	}
	return result, nil
}

// ModelCredential returns cloud credential information for a  model.
func (api *CredentialValidatorAPI) ModelCredential() (params.ModelCredential, error) {
	c, err := api.backend.ModelCredential()
	if err != nil {
		return params.ModelCredential{}, common.ServerError(err)
	}

	return params.ModelCredential{
		Model:           c.Model.String(),
		CloudCredential: c.Credential.String(),
		Exists:          c.Exists,
		Valid:           c.Valid,
	}, nil
}

// Mask out new methods from the old API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
//
// WatchModelCredential did not exist prior to v2.
func (*CredentialValidatorAPIV1) WatchModelCredential(_, _ struct{}) {}

// WatchModelCredential returns a NotifyWatcher that watches what cloud credential a model uses.
func (api *CredentialValidatorAPI) WatchModelCredential() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch, err := api.backend.WatchModelCredential()
	if err != nil {
		return result, common.ServerError(err)
	}

	// Consume the initial event. Technically, API calls to Watch
	// 'transmit' the initial event in the Watch response. But
	// NotifyWatchers have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = api.resources.Register(watch)
	} else {
		err = watcher.EnsureErr(watch)
		result.Error = common.ServerError(err)
	}
	return result, nil
}
