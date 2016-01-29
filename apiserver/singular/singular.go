// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade(
		"Singular", 1,
		func(st *state.State, _ *common.Resources, auth common.Authorizer) (*Facade, error) {
			return NewFacade(st, auth)
		},
	)
}

// Backend supplies capabilities required by a Facade.
type Backend interface {

	// ModelTag tells the Facade what models it should consider requests for.
	ModelTag() names.ModelTag

	// SingularClaimer allows the Facade to make claims.
	SingularClaimer() lease.Claimer
}

// NewFacade returns a singular-controller API facade, backed by the supplied
// state, so long as the authorizer represents a controller machine.
func NewFacade(backend Backend, auth common.Authorizer) (*Facade, error) {
	if !auth.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &Facade{
		auth:    auth,
		model:   backend.ModelTag(),
		claimer: backend.SingularClaimer(),
	}, nil
}

// Facade allows controller machines to request exclusive rights to administer
// some specific model for a limited time.
type Facade struct {
	auth    common.Authorizer
	model   names.ModelTag
	claimer lease.Claimer
}

// Wait waits for the singular-controller lease to expire for all supplied
// entities. (In practice, any requests that do not refer to the connection's
// model will be rejected.)
func (facade *Facade) Wait(args params.Entities) (result params.ErrorResults) {
	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		var err error
		switch {
		case entity.Tag != facade.model.String():
			err = common.ErrPerm
		default:
			err = facade.claimer.WaitUntilExpired(facade.model.Id())
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result
}

// Claim makes the supplied singular-controller lease requests. (In practice,
// any requests not for the connection's model, or not on behalf of the
// connected EnvironManager machine, will be rejected.)
func (facade *Facade) Claim(args params.SingularClaims) (result params.ErrorResults) {
	result.Results = make([]params.ErrorResult, len(args.Claims))
	for i, claim := range args.Claims {
		var err error
		switch {
		case claim.ModelTag != facade.model.String():
			err = common.ErrPerm
		case claim.ControllerTag != facade.auth.GetAuthTag().String():
			err = common.ErrPerm
		case !allowedDuration(claim.Duration):
			err = common.ErrPerm
		default:
			err = facade.claimer.Claim(facade.model.Id(), claim.ControllerTag, claim.Duration)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result
}

// allowedDuration returns true if the supplied duration is at least one second,
// and no more than one minute. (We expect to refine the lease-length times, but
// these seem like reasonable bounds.)
func allowedDuration(duration time.Duration) bool {
	if duration < time.Second {
		return false
	}
	return duration <= time.Minute
}
