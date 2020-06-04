// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package verifycharmprofile

import (
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

type verifyCharmProfileResolver struct {
	logger Logger
}

// NewResolver returns a new verify charm profile resolver.
func NewResolver(logger Logger) resolver.Resolver {
	return &verifyCharmProfileResolver{logger: logger}
}

// NextOp is defined on the Resolver interface.
// This NextOp is only meant to be called before any Upgrade operation.
func (r *verifyCharmProfileResolver) NextOp(
	_ resolver.LocalState, remoteState remotestate.Snapshot, _ operation.Factory,
) (operation.Operation, error) {
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
	if rev != remoteState.CharmURL.Revision {
		r.logger.Tracef("Charm profile required: current revision %d does not match new revision %d", rev, remoteState.CharmURL.Revision)
		return nil, resolver.ErrDoNotProceed
	}
	r.logger.Tracef("Charm profile correct for charm revision")
	return nil, resolver.ErrNoOperation
}
