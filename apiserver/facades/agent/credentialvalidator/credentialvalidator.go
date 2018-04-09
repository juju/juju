// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package credentialvalidator

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.api.credentialvalidator")

// CredentialValidator defines the methods on credentialvalidator API endpoint.
type CredentialValidator interface {
	ModelCredentials(params.Entities) (params.ModelCredentialResults, error)
	WatchCredential(params.Entities) (params.NotifyWatchResults, error)
}

type CredentialValidatorAPI struct {
	backend   Backend
	resources facade.Resources
}

var _ CredentialValidator = (*CredentialValidatorAPI)(nil)

// NewCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func NewCredentialValidatorAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(NewStateBackend(st), resources, authorizer)
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

// auth only accepts the model tag reported by the backend.
func (api *CredentialValidatorAPI) auth(tagString string) error {
	tag, err := names.ParseModelTag(tagString)
	if err != nil {
		return errors.Trace(err)
	}
	if tag.Id() != api.backend.ModelUUID() {
		return common.ErrPerm
	}
	return nil
}

// WatchCredential returns a collection of NotifyWatchers that observe
// changes to the given cloud credentials.
// The order of returned watchers is important and corresponds directly to the
// order of supplied cloud credentials collection.
func (api *CredentialValidatorAPI) WatchCredential(args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		credentialTag, err := names.ParseCloudCredentialTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		// Is credential used by the model that has created this backend?
		// Technically just as with above, this will mean that only 1 credential will ever
		// be ok in this collection...
		isUsed, err := api.backend.ModelUsesCredential(credentialTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !isUsed {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		watch := api.backend.WatchCredential(credentialTag)
		// Consume the initial event. Technically, API calls to Watch
		// 'transmit' the initial event in the Watch response. But
		// NotifyWatchers have no state to transmit.
		if _, ok := <-watch.Changes(); ok {
			results.Results[i].NotifyWatcherId = api.resources.Register(watch)
		} else {
			err = watcher.EnsureErr(watch)
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// ModelCredentials returns cloud credential information for each
// given model. For models that are on the clouds that do not require cloud
// credentials, the result will have Exists set to false.
// It is also possible to encounter some processing errors for a model in a collection.
// This can be returned instead of its cloud credential.
// The order of returned cloud credential information is important and
// corresponds directly to the order of supplied models collection.
func (api *CredentialValidatorAPI) ModelCredentials(args params.Entities) (params.ModelCredentialResults, error) {
	results := params.ModelCredentialResults{
		Results: make([]params.ModelCredentialResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		mc, err := api.modelCredential(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = mc
	}
	return results, nil
}

// modelCredential does auth and lookup for a single entity.
func (api *CredentialValidatorAPI) modelCredential(tagString string) (*params.ModelCredential, error) {
	if err := api.auth(tagString); err != nil {
		return nil, errors.Trace(err)
	}
	c, err := api.backend.ModelCredential()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &params.ModelCredential{
		Model:           c.Model.String(),
		CloudCredential: c.Credential.String(),
		Exists:          c.Exists,
		// TODO (anastasiamac 2018-04-06) Expand this to mean "valid for this model"...
		Valid: c.Valid,
	}, nil
}
