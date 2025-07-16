// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/internal/network/containerizer"
)

// Machine is an indirection for use in container provisioning.
// It is an indirection for both containerizer.Machine and
// containerizer.Container as well as state.Machine locally.
type Machine interface {
	containerizer.Container

	MachineTag() names.MachineTag
}
