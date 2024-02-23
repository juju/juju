// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile

import (
	"context"

	jujucharm "github.com/juju/juju/charm"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

type verifyCharmProfileResolver struct {
	logger Logger
}

// NewResolver returns a new verify charm profile resolver.
func NewResolver(logger Logger, modelType model.ModelType) resolver.Resolver {
	if modelType == model.CAAS {
		return &caasVerifyCharmProfileResolver{}
	}
	return &verifyCharmProfileResolver{logger: logger}
}

// NextOp is defined on the Resolver interface.
// This NextOp is only meant to be called before any Upgrade operation.
func (r *verifyCharmProfileResolver) NextOp(
	ctx context.Context,
	_ resolver.LocalState, remoteState remotestate.Snapshot, _ operation.Factory,
) (operation.Operation, error) {
	// NOTE: this is very similar code to Uniter.verifyCharmProfile(),
	// if you make changes here, check to see if they are needed there.
	r.logger.Tracef("Starting verifycharmprofile NextOp")
	if !remoteState.CharmProfileRequired {
		r.logger.Tracef("Nothing to verify: no charm profile required")
		return nil, resolver.ErrNoOperation
	}
	if remoteState.LXDProfileName == "" {
		r.logger.Tracef("Charm profile required: no profile for this charm applied")
		return nil, resolver.ErrDoNotProceed
	}
	rev, err := lxdprofile.ProfileRevision(remoteState.LXDProfileName)
	if err != nil {
		return nil, err
	}
	curl, err := jujucharm.ParseURL(remoteState.CharmURL)
	if err != nil {
		return nil, err
	}
	if rev != curl.Revision {
		r.logger.Tracef("Charm profile required: current revision %d does not match new revision %d", rev, curl.Revision)
		return nil, resolver.ErrDoNotProceed
	}
	r.logger.Tracef("Charm profile correct for charm revision")
	return nil, resolver.ErrNoOperation
}

type caasVerifyCharmProfileResolver struct{}

// NextOp is defined on the Resolver interface.
// This NextOp ensures that we never check for lxd profiles on CAAS machines.
func (r *caasVerifyCharmProfileResolver) NextOp(
	_ context.Context,
	_ resolver.LocalState, _ remotestate.Snapshot, _ operation.Factory,
) (operation.Operation, error) {
	return nil, resolver.ErrNoOperation
}
