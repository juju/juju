// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

var logger = loggo.GetLogger("juju.api.credentialvalidator")

// Facade provides methods that the Juju client command uses to interact
// with the Juju backend.
type Facade struct {
	facade base.FacadeCaller
}

// NewFacade creates a new `Facade` based on an existing authenticated API
// connection.
func NewFacade(caller base.APICaller) *Facade {
	return &Facade{base.NewFacadeCaller(caller, "CredentialValidator")}
}

// ModelCredential gets the cloud credential that a given model uses, including
// useful data such as "is this credential valid"...
// Some clouds do not require a credential and support the "empty" authentication
// type. Models on these clouds will have no credentials set, and thus, will return
// a false as 2nd argument.
func (c *Facade) ModelCredential(modelUUID string) (base.StoredCredential, bool, error) {
	// Construct model tag from model uuid.
	in := params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(modelUUID).String()}},
	}

	// Call apiserver to get credential for this model.
	out := params.ModelCredentialResults{}
	emptyResult := base.StoredCredential{}
	if err := c.facade.FacadeCall("ModelCredentials", in, &out); err != nil {
		return emptyResult, false, errors.Trace(err)
	}

	// There should be just 1.
	if count := len(out.Results); count != 1 {
		return emptyResult, false, errors.Errorf("expected 1 model credential for model %q, got %d", modelUUID, count)
	}
	if err := out.Results[0].Error; err != nil {
		return emptyResult, false, errors.Trace(err)
	}

	result := out.Results[0].Result

	modelTag, err := names.ParseModelTag(result.Model)
	if err != nil {
		return emptyResult, false, errors.Trace(err)
	}
	if modelTag.Id() != modelUUID {
		return emptyResult, false, errors.Errorf("unexpected credential for model %q, expected credential for model %q", modelTag.Id(), modelUUID)
	}

	if !result.Exists {
		return emptyResult, false, nil
	}

	credentialTag, err := names.ParseCloudCredentialTag(result.CloudCredential)
	if err != nil {
		return emptyResult, false, errors.Trace(err)
	}
	return base.StoredCredential{
		CloudCredential: credentialTag.Id(),
		Valid:           result.Valid,
	}, true, nil
}

// WatchCredential provides notify watcher that is responsive to changes
// to a given cloud credential.
func (c *Facade) WatchCredential(credentialID string) (watcher.NotifyWatcher, error) {
	// Construct credential tag from given id.
	in := params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudCredentialTag(credentialID).String()}},
	}

	// Call apiserver to get the watcher for this credential.
	var results params.NotifyWatchResults
	err := c.facade.FacadeCall("WatchCredential", in, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// There should be just 1.
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("expected 1 watcher for credential %q, got %d", credentialID, count)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}
