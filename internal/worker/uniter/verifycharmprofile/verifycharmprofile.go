// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	jujucharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type verifyCharmProfileResolver struct {
	logger logger.Logger
}

// NewResolver returns a new verify charm profile resolver.
func NewResolver(logger logger.Logger, modelType model.ModelType) resolver.Resolver {
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
	r.logger.Tracef(context.TODO(), "Starting verifycharmprofile NextOp")
	if !remoteState.CharmProfileRequired {
		r.logger.Tracef(context.TODO(), "Nothing to verify: no charm profile required")
		return nil, resolver.ErrNoOperation
	}
	if remoteState.LXDProfileName == "" {
		r.logger.Tracef(context.TODO(), "Charm profile required: no profile for this charm applied")
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
		r.logger.Tracef(context.TODO(), "Charm profile required: current revision %d does not match new revision %d", rev, curl.Revision)
		return nil, resolver.ErrDoNotProceed
	}
	r.logger.Tracef(context.TODO(), "Charm profile correct for charm revision")
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
