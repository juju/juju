// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package placement

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/errors"
)

// PlacementType is the type of placement.
type PlacementType int

const (
	// PlacementTypeUnset is the type of placement for unset.
	PlacementTypeUnset PlacementType = iota
	// PlacementTypeMachine is the type of placement for machines.
	PlacementTypeMachine
	// PlacementTypeContainer is the type of placement for containers.
	PlacementTypeContainer
	// PlacementTypeProvider is the type of placement for instances.
	PlacementTypeProvider
)

// ContainerType is the type of container.
type ContainerType int

const (
	// ContainerTypeNone is the type for no container.
	ContainerTypeNone ContainerType = iota
	// ContainerTypeLXD is the type for LXD containers.
	ContainerTypeLXD
)

// Placement is the placement of an application.
type Placement struct {
	Type      PlacementType
	Container ContainerType
	Directive string
}

// ParsePlacement parses the placement from the instance placement.
func ParsePlacement(placement *instance.Placement) (Placement, error) {
	// If no placement is present, we assume that a machine placement will
	// be used.
	if placement == nil {
		return Placement{
			Type: PlacementTypeUnset,
		}, nil
	}

	// If the placement is not empty, try and parse the scope to determine
	// the type of placement.
	if container, err := instance.ParseContainerType(placement.Scope); err == nil {
		containerType, err := parseContainerType(container)
		if err != nil {
			return Placement{}, err
		}

		return Placement{
			Type:      PlacementTypeContainer,
			Container: containerType,
		}, nil
	}

	// It's not a container type, so we check if it's a machine scope or
	// a provider assigned placement.
	switch placement.Scope {
	case instance.ModelScope:
		return Placement{
			Type:      PlacementTypeProvider,
			Directive: placement.Directive,
		}, nil
	case instance.MachineScope:
		return Placement{
			Type:      PlacementTypeMachine,
			Directive: placement.Directive,
		}, nil
	default:
		return Placement{}, errors.Errorf("placement scope %q not supported", placement.Scope)
	}
}

func parseContainerType(containerType instance.ContainerType) (ContainerType, error) {
	switch containerType {
	case instance.LXD:
		return ContainerTypeLXD, nil
	case instance.NONE:
		return ContainerTypeNone, nil
	default:
		return 0, errors.Errorf("container type %q not supported", containerType)
	}
}
