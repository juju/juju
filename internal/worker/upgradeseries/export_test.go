// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import "github.com/juju/juju/core/base"

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchHostBase(patcher patcher, b base.Base) {
	patcher.PatchValue(&hostBase, func() (base.Base, error) { return b, nil })
}
