// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	jujucharm "github.com/juju/charm/v7"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/network/containerizer"
)

// Machine is an indirection for use in container provisioning.
// It is an indirection for both containerizer.Machine and
// containerizer.Container as well as state.Machine locally.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/agent/provisioner Machine,BridgePolicy,Unit,Application,Charm
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/containerizer_mock.go github.com/juju/juju/network/containerizer LinkLayerDevice
type Machine interface {
	containerizer.Container

	Units() ([]Unit, error)
	InstanceId() (instance.Id, error)
	IsManual() (bool, error)
	MachineTag() names.MachineTag
}

// BridgePolicy is an indirection for containerizer.BridgePolicy.
type BridgePolicy interface {
	// FindMissingBridgesForContainer looks at the spaces that the container should
	// have access to, and returns any host devices need to be bridged for use as
	// the container network.
	FindMissingBridgesForContainer(containerizer.Machine, containerizer.Container) ([]network.DeviceToBridge, int, error)

	// PopulateContainerLinkLayerDevices sets the link-layer devices of the input
	// guest, setting each device to be a child of the corresponding bridge on the
	// host machine.
	PopulateContainerLinkLayerDevices(containerizer.Machine, containerizer.Container) error
}

// Unit is an indirection for state.Unit.
type Unit interface {
	Application() (Application, error)
	Name() string
}

// Application is an indirection for state.Application.
type Application interface {
	Charm() (Charm, bool, error)
	Name() string
}

// Charm is an indirection for state.Charm.
type Charm interface {
	LXDProfile() *jujucharm.LXDProfile
	Revision() int
}
