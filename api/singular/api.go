// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lease"
)

// NewAPI returns a new API client for the Singular facade. It exposes methods
// for claiming and observing administration responsibility for the entity with
// the supplied tag, on behalf of the authenticated agent.
func NewAPI(
	apiCaller base.APICaller,
	claimant names.Tag,
	entity names.Tag,
) (*API, error) {
	if !names.IsValidMachine(claimant.Id()) && !names.IsValidControllerAgent(claimant.Id()) {
		return nil, errors.NotValidf("claimant tag")
	}
	switch entity.(type) {
	case names.ModelTag:
	case names.ControllerTag:
	case nil:
		return nil, errors.New("nil entity supplied")
	default:
		return nil, errors.Errorf(
			"invalid entity kind %q for singular API", entity.Kind(),
		)
	}
	facadeCaller := base.NewFacadeCaller(apiCaller, "Singular")
	return &API{
		facadeCaller: facadeCaller,
		claimant:     claimant,
		entity:       entity,
	}, nil
}

// API allows controller machines to claim responsibility for; or to wait for
// no other machine to have responsibility for; administration for some model.
type API struct {
	facadeCaller base.FacadeCaller
	claimant     names.Tag
	entity       names.Tag
}

// Claim attempts to claim responsibility for administration of the entity
// for the supplied duration. If the claim is denied, it will return
// lease.ErrClaimDenied.
func (api *API) Claim(duration time.Duration) error {
	args := params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   api.entity.String(),
			ClaimantTag: api.claimant.String(),
			Duration:    duration,
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

// Wait blocks until nobody has responsibility for administration of the
// entity. It should probably be doing something watchy rather than blocky,
// but it's following the lease manager implementation underlying the original
// leadership approach and it doesn't seem worth rewriting all that.
func (api *API) Wait() error {
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: api.entity.String(),
		}},
	}
	var results params.ErrorResults
	err := api.facadeCaller.FacadeCall("Wait", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
