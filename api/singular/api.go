// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lease"
)

// NewAPI returns a new API client for the Singular facade. It exposes methods
// for claiming and observing administration responsibility for the apiCaller's
// model, on behalf of the supplied controller machine.
func NewAPI(apiCaller base.APICaller, controllerTag names.MachineTag) (*API, error) {
	controllerId := controllerTag.Id()
	if !names.IsValidMachine(controllerId) {
		return nil, errors.NotValidf("controller tag")
	}
	modelTag, err := apiCaller.ModelTag()
	if err != nil {
		return nil, errors.Trace(err)
	}
	facadeCaller := base.NewFacadeCaller(apiCaller, "Singular")
	return &API{
		modelTag:      modelTag,
		controllerTag: controllerTag,
		facadeCaller:  facadeCaller,
	}, nil
}

// API allows controller machines to claim responsibility for; or to wait for
// no other machine to have responsibility for; administration for some model.
type API struct {
	modelTag      names.ModelTag
	controllerTag names.MachineTag
	facadeCaller  base.FacadeCaller
}

// Claim attempts to claim responsibility for model administration for the
// supplied duration. If the claim is denied, it will return
// lease.ErrClaimDenied.
func (api *API) Claim(duration time.Duration) error {
	args := params.SingularClaims{
		Claims: []params.SingularClaim{{
			ModelTag:      api.modelTag.String(),
			ControllerTag: api.controllerTag.String(),
			Duration:      duration,
		}},
	}
	var results params.ErrorResults
	err := api.facadeCaller.FacadeCall("Claim", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	err = results.OneError()
	if err != nil {
		if params.IsCodeLeaseClaimDenied(err) {
			return lease.ErrClaimDenied
		}
		return errors.Trace(err)
	}
	return nil
}

// Wait blocks until nobody has responsibility for model administration. It
// should probably be doing something watchy rather than blocky, but it's
// following the lease manager implementation underlying the original
// leadership approach and it doesn't seem worth rewriting all that.
func (api *API) Wait() error {
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: api.modelTag.String(),
		}},
	}
	var results params.ErrorResults
	err := api.facadeCaller.FacadeCall("Wait", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
