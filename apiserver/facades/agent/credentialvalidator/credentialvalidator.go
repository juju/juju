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
	ModelCredential() (params.ModelCredential, error)
	WatchCredential(string) (params.NotifyWatchResult, error)
}

type CredentialValidatorAPI struct {
	backend   Backend
	resources facade.Resources
}

var _ CredentialValidator = (*CredentialValidatorAPI)(nil)

// NewCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func NewCredentialValidatorAPI(ctx facade.Context) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(NewBackend(ctx.State()), ctx.Resources(), ctx.Auth())
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

// WatchCredential returns a collection of NotifyWatchers that observe
// changes to the given cloud credentials.
// The order of returned watchers is important and corresponds directly to the
// order of supplied cloud credentials collection.
func (api *CredentialValidatorAPI) WatchCredential(tag string) (params.NotifyWatchResult, error) {
	fail := func(failure error) (params.NotifyWatchResult, error) {
		return params.NotifyWatchResult{}, common.ServerError(failure)
	}

	credentialTag, err := names.ParseCloudCredentialTag(tag)
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
