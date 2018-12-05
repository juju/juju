// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradecharmprofile

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

var logger = loggo.GetLogger("juju.worker.uniter.upgradecharmprofile")

type upgradeCharmProfileResolver struct{}

// NewResolver returns a new upgrade charm profile resolver
func NewResolver() resolver.Resolver {
	return &upgradeCharmProfileResolver{}
}

// NextOp is defined on the Resolver interface.
func (l *upgradeCharmProfileResolver) NextOp(
	localState resolver.LocalState, remoteState remotestate.Snapshot, opFactory operation.Factory,
) (operation.Operation, error) {
	// Ensure the lxd profile is installed, before we move to upgrading
	// of the charm.
	if !lxdprofile.UpgradeStatusTerminal(remoteState.UpgradeCharmProfileStatus) {
		return nil, resolver.ErrDoNotProceed
	}
	// If the upgrade status is in an error state, we should log it out
	// to the operator.
	if lxdprofile.UpgradeStatusErrorred(remoteState.UpgradeCharmProfileStatus) {
		// This is a terminal error for the machine, as we can no longer promise
		// to deliver the correct lxd profile on the machine; one which the
		// operator asked for. Instead we need to error out and allow the
		// operator to update the lxd profile before attempting to re-apply the
		// charm.
		if localState.UpgradeCharmProfileStatus == lxdprofile.NotKnownStatus {
			return nil, resolver.ErrNoOperation
		}
		logger.Errorf("error upgrading charm profile: %v", remoteState.UpgradeCharmProfileStatus)
		return opFactory.NewFinishUpgradeCharmProfile(remoteState.CharmURL)
	}

	return nil, resolver.ErrNoOperation
}
