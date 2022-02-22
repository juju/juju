// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
)

func NewMachine(caller base.APICaller, tag names.MachineTag, life life.Value) *Machine {
	facade := base.NewFacadeCaller(caller, instancePollerFacade)
	return &Machine{facade, tag, life}
}

var NewStringsWatcher = &newStringsWatcher
