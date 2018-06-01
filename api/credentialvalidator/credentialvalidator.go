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
func (c *Facade) ModelCredential() (base.StoredCredential, bool, error) {
	out := params.ModelCredential{}
	emptyResult := base.StoredCredential{}
	if err := c.facade.FacadeCall("ModelCredential", nil, &out); err != nil {
		return emptyResult, false, errors.Trace(err)
	}

	if !out.Exists {
		return emptyResult, false, nil
	}

	credentialTag, err := names.ParseCloudCredentialTag(out.CloudCredential)
	if err != nil {
		return emptyResult, false, errors.Trace(err)
	}
	return base.StoredCredential{
		CloudCredential: credentialTag.Id(),
		Valid:           out.Valid,
	}, true, nil
}

// WatchCredential provides a notify watcher that is responsive to changes
// to a given cloud credential.
func (c *Facade) WatchCredential(credentialID string) (watcher.NotifyWatcher, error) {
	in := names.NewCloudCredentialTag(credentialID).String()
	var result params.NotifyWatchResult
	err := c.facade.FacadeCall("WatchCredential", params.Entity{in}, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := result.Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// InvalidateModelCredential invalidates cloud credential for the model that made a connection.
func (c *Facade) InvalidateModelCredential(reason string) error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("InvalidateModelCredential", reason, &result)
	if err != nil {
		return errors.Trace(err)
	}

	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}
