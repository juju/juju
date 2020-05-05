// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
)

func NewMachine(facadeCaller base.FacadeCaller, tag names.MachineTag, life life.Value) *Machine {
	return &Machine{
		facade: facadeCaller,
		tag:    tag,
		life:   life,
	}
}
