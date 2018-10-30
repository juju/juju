// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/model"
)

// LXDProfileUpgradeStatus returns the lxd profile upgrade status.
func (m *Machine) LXDProfileUpgradeStatus() (model.LXDProfileUpgradeStatus, error) {
	// TODO: (Simon) - how do we get this back?
	return "", nil
}

// LXDProfileUpgradeUnitStatus returns the lxd profile upgrade status for the
// input unit.
func (m *Machine) LXDProfileUpgradeUnitStatus(unitName string) (model.LXDProfileUpgradeStatus, error) {
	// TODO: (Simon) - how do we get this back?
	return "", nil
}
