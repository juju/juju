// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/core/watcher"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Machine represents a juju machine as seen by an instancemutater
// worker.
type Machine struct {
	facade base.FacadeCaller

	tag  names.MachineTag
	life params.Life
}

func (m *Machine) CharmProfiles() ([]string, error) {
	return nil, nil
}

func (m *Machine) SetUpgradeCharmProfileComplete(unitName string, message string) error {
	return nil
}

func (m *Machine) Tag() names.MachineTag {
	return m.tag
}

func (m *Machine) WatchUnits() (watcher.StringsWatcher, error) {
	return nil, nil
}
