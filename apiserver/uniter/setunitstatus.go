// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

type UnitStatusSetter struct {
	*common.StatusSetter
}

// NewUnitStatusSetter returns a new UnitStatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewUnitStatusSetter(st state.EntityFinder, getCanModify common.GetAuthFunc) *UnitStatusSetter {
	statusSetter := common.NewStatusSetter(
		st,
		getCanModify)
	return &UnitStatusSetter{statusSetter}
}
