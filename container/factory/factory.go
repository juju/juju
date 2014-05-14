// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This package exists solely to avoid circular imports.

package factory

import (
	"fmt"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/instance"
)

// NewContainerManager creates the appropriate container.Manager for the
// specified container type.
func NewContainerManager(forType instance.ContainerType, conf container.ManagerConfig) (container.Manager, error) {
	switch forType {
	case instance.LXC:
		return lxc.NewContainerManager(conf)
	case instance.KVM:
		return kvm.NewContainerManager(conf)
	}
	return nil, fmt.Errorf("unknown container type: %q", forType)
}
