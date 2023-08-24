// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"github.com/juju/juju/cloudconfig"
	corebase "github.com/juju/juju/core/base"
)

var (
	ResolvConfFiles       = &resolvConfFiles
	CombinedCloudInitData = combinedCloudInitData
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

// PatchNewMachineInitReader replaces the local init reader factory method
// with the supplied one.
func PatchNewMachineInitReader(patcher patcher, factory func(base corebase.Base) (cloudconfig.InitReader, error)) {
	patcher.PatchValue(&newMachineInitReader, factory)
}
