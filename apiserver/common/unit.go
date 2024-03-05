// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/leadership"
)

// RevokeLeadershipFunc returns a function that revokes leadership for dead units.
func RevokeLeadershipFunc(leadershipRevoker leadership.Revoker) func(names.Tag) {
	return func(tag names.Tag) {
		if tag.Kind() != names.UnitTagKind {
			return
		}
		appName, _ := names.UnitApplication(tag.Id())
		if err := leadershipRevoker.RevokeLeadership(appName, tag.Id()); err != nil && err != leadership.ErrClaimNotHeld {
			logger.Warningf("cannot revoke lease for dead unit %q", tag.Id())
		}
	}
}
