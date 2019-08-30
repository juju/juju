// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

func NewMachine(facadeCaller base.FacadeCaller, tag names.MachineTag, life params.Life) *Machine {
	return &Machine{
		facade: facadeCaller,
		tag:    tag,
		life:   life,
	}
}
