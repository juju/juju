// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/state"
)

// NewExternalFacade is for API registration.
func NewExternalFacade(context facade.Context) (*Facade, error) {
	st := context.State()
	auth := context.Auth()

	m, err := st.Model()
	if err != nil {
		return nil, err
	}

	claimer, err := context.SingularClaimer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend := getBackend(st, m.ModelTag())
	return NewFacade(backend, claimer, auth)
}

var getBackend = func(st *state.State, modelTag names.ModelTag) Backend {
	return &stateBackend{st, modelTag}
}

type stateBackend struct {
	*state.State
	modelTag names.ModelTag
}

// ModelTag is part of the Backend interface.
func (b *stateBackend) ModelTag() names.ModelTag {
	return b.modelTag
}

// Backend supplies capabilities required by a Facade.
type Backend interface {
	// ControllerTag tells the Facade which controller it should consider
	// requests for.
	ControllerTag() names.ControllerTag

	// ModelTag tells the Facade what models it should consider requests for.
	ModelTag() names.ModelTag
}

// NewFacade returns a singular-controller API facade, backed by the supplied
// state, so long as the authorizer represents a controller machine.
func NewFacade(backend Backend, claimer lease.Claimer, auth facade.Authorizer) (*Facade, error) {
	if !auth.AuthController() {
		return nil, common.ErrPerm
	}
	return &Facade{
		auth:            auth,
		modelTag:        backend.ModelTag(),
		controllerTag:   backend.ControllerTag(),
		singularClaimer: claimer,
	}, nil
}

// Facade allows controller machines to request exclusive rights to administer
// some specific model or controller for a limited time.
type Facade struct {
	auth            facade.Authorizer
	controllerTag   names.ControllerTag
	modelTag        names.ModelTag
	singularClaimer lease.Claimer
}

// Wait waits for the singular-controller lease to expire for all supplied
// entities. (In practice, any requests that do not refer to the connection's
// model or controller will be rejected.)
func (facade *Facade) Wait(ctx context.Context, args params.Entities) (result params.ErrorResults) {
	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		leaseId, err := facade.tagLeaseId(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		// TODO(axw) 2017-10-30 #1728594
		// We should be waiting for the leases in parallel,
		// so the waits do not affect one another.
		err = facade.singularClaimer.WaitUntilExpired(leaseId, ctx.Done())
		result.Results[i].Error = common.ServerError(err)
	}
	return result
}

// Claim makes the supplied singular-controller lease requests. (In practice,
// any requests not for the connection's model or controller, or not on behalf
// of the connected ModelManager machine, will be rejected.)
func (facade *Facade) Claim(args params.SingularClaims) (result params.ErrorResults) {
	result.Results = make([]params.ErrorResult, len(args.Claims))
	for i, claim := range args.Claims {
		err := facade.claim(claim)
		result.Results[i].Error = common.ServerError(err)
	}
	return result
}

func (facade *Facade) claim(claim params.SingularClaim) error {
	if !allowedDuration(claim.Duration) {
		return common.ErrPerm
	}
	leaseId, err := facade.tagLeaseId(claim.EntityTag)
	if err != nil {
		return errors.Trace(err)
	}
	holder := facade.auth.GetAuthTag().String()
	if claim.ClaimantTag != holder {
		return common.ErrPerm
	}
	return facade.singularClaimer.Claim(leaseId, holder, claim.Duration)
}

func (facade *Facade) tagLeaseId(tagString string) (string, error) {
	tag, err := names.ParseTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	switch tag {
	case facade.modelTag, facade.controllerTag:
		return tag.Id(), nil
	}
	return "", common.ErrPerm
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
