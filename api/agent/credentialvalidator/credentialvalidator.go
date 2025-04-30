// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Facade provides methods that the Juju client command uses to interact
// with the Juju backend.
type Facade struct {
	facade base.FacadeCaller
}

// NewFacade creates a new `Facade` based on an existing authenticated API
// connection.
func NewFacade(caller base.APICaller, options ...Option) *Facade {
	return &Facade{base.NewFacadeCaller(caller, "CredentialValidator", options...)}
}

// ModelCredential gets the cloud credential that a given model uses, including
// useful data such as "is this credential valid"...
// Some clouds do not require a credential and support the "empty" authentication
// type. Models on these clouds will have no credentials set, and thus, will return
// a false as 2nd argument.
func (c *Facade) ModelCredential(ctx context.Context) (base.StoredCredential, bool, error) {
	out := params.ModelCredential{}
	emptyResult := base.StoredCredential{}
	if err := c.facade.FacadeCall(ctx, "ModelCredential", nil, &out); err != nil {
		return emptyResult, false, errors.Trace(err)
	}

	if !out.Exists {
		// On some clouds, model credential may not be required.
		// So, it may be valid for models to not have a credential set.
		return base.StoredCredential{Valid: out.Valid}, false, nil
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

// InvalidateModelCredential invalidates cloud credential for the model that made a connection.
func (c *Facade) InvalidateModelCredential(ctx context.Context, reason string) error {
	in := params.InvalidateCredentialArg{reason}
	var result params.ErrorResult
	err := c.facade.FacadeCall(ctx, "InvalidateModelCredential", in, &result)
	if err != nil {
		return errors.Trace(err)
	}

	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// WatchModelCredential provides a notify watcher that is responsive to changes
// to a given cloud credential.
func (c *Facade) WatchModelCredential(ctx context.Context) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.facade.FacadeCall(ctx, "WatchModelCredential", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := result.Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
