// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployment

import (
	"strings"

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
	// ContainerTypeUnknown is the type for no container.
	ContainerTypeUnknown ContainerType = iota
	// ContainerTypeLXD is the type for LXD containers.
	ContainerTypeLXD
)

func (containerType ContainerType) String() string {
	switch containerType {
	case ContainerTypeLXD:
		return "lxd"
	default:
		return "unknown"
	}
}

// Placement is the placement of an application.
type Placement struct {
	// Type is the type of placement.
	Type PlacementType
	// Container is the type of container (lxd for example) that we're going
	// to associate a unit with.
	Container ContainerType
	// Directive is the raw placement directive. This will change depending on
	// the type.
	// - This will be empty if the placement is unset.
	// - This will be the machine name (0 or 0/lxd/0) if the placement is a
	//   machine.
	// - This will be empty if the placement is a container.
	// - For model scope, this will be the provider directive. This is up to
	//   the provider to interpret.
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

	switch placement.Scope {
	case instance.ModelScope:
		return Placement{
			Type:      PlacementTypeProvider,
			Directive: placement.Directive,
		}, nil

	case instance.MachineScope:
		if err := parseMachineParenting(placement.Directive); err != nil {
			return Placement{}, errors.Capture(err)
		}

		return Placement{
			Type:      PlacementTypeMachine,
			Directive: placement.Directive,
		}, nil

	default:
		container, err := instance.ParseContainerType(placement.Scope)
		if err != nil {
			return Placement{}, errors.Capture(err)
		} else if placement.Directive != "" {
			return Placement{}, errors.Errorf("placement directive %q is not supported for container type %q", placement.Directive, placement.Scope)
		}

		containerType, err := parseContainerType(container)
		if err != nil {
			return Placement{}, err
		}

		return Placement{
			Type:      PlacementTypeContainer,
			Container: containerType,
		}, nil
	}
}

func parseContainerType(containerType instance.ContainerType) (ContainerType, error) {
	switch containerType {
	case instance.LXD:
		return ContainerTypeLXD, nil
	default:
		return ContainerTypeUnknown, errors.Errorf("container type %q not supported", containerType)
	}
}

func parseMachineParenting(directive string) error {
	if directive == "" {
		return errors.Errorf("placement directive %q cannot be empty", directive)
	}

	// We prevent an unbounded number of slashes by limiting the split to 4
	// parts. This allows us to provide a more specific error message if the
	// directive is not in the form of <parent>/<scope>/<child>.
	parts := strings.SplitN(directive, "/", 4)
	switch len(parts) {
	case 1:
		// There is no parenting directive, so there is no need to parse
		// anything.
		return nil
	case 3:
		// This should be in the form of `<parent>/<scope>/<child>`.
		_, err := parseContainerType(instance.ContainerType(parts[1]))
		return err
	default:
		return errors.Errorf("placement directive %q is not in the form of <parent>/<scope>/<child>", directive)
	}
}
