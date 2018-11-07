// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.api.credentialvalidator")

// CredentialValidator defines the methods on credentialvalidator API endpoint.
type CredentialValidator interface {
	InvalidateModelCredential(args params.InvalidateCredentialArg) (params.ErrorResult, error)
	ModelCredential() (params.ModelCredential, error)
	WatchCredential(params.Entity) (params.NotifyWatchResult, error)
}

type CredentialValidatorAPI struct {
	backend   Backend
	resources facade.Resources
}

var _ CredentialValidator = (*CredentialValidatorAPI)(nil)

// NewCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func NewCredentialValidatorAPI(ctx facade.Context) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(NewBackend(NewStateShim(ctx.State())), ctx.Resources(), ctx.Auth())
}

func internalNewCredentialValidatorAPI(backend Backend, resources facade.Resources, authorizer facade.Authorizer) (*CredentialValidatorAPI, error) {
	hostAuthTag := authorizer.GetAuthTag()
	if hostAuthTag == nil {
		return nil, common.ErrPerm
	}

	return &CredentialValidatorAPI{
		resources: resources,
		backend:   backend,
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

// InvalidateModelCredential marks the cloud credential for this model as invalid.
func (api *CredentialValidatorAPI) InvalidateModelCredential(args params.InvalidateCredentialArg) (params.ErrorResult, error) {
	err := api.backend.InvalidateModelCredential(args.Reason)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}
