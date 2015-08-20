// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// DeployServiceParams contains the arguments required to deploy the referenced
// charm.
type DeployServiceParams struct {
	ServiceName    string
	ServiceOwner   string
	Charm          *state.Charm
	ConfigSettings charm.Settings
	Constraints    constraints.Value
	NumUnits       int
	// ToMachineSpec is either:
	// - an existing machine/container id eg "1" or "1/lxc/2"
	// - a new container on an existing machine eg "lxc:1"
	// Use string to avoid ambiguity around machine 0.
	ToMachineSpec string
	// Placement is a list of placement directives which may be used
	// instead of a machine spec.
	Placement []*instance.Placement
	// Networks holds a list of networks to required to start on boot.
	// TODO(dimitern): Drop this in a follow-up in favor of constraints.
	Networks []string
	Storage  map[string]storage.Constraints
}

// DeployService takes a charm and various parameters and deploys it.
func DeployService(st *state.State, args DeployServiceParams) (*state.Service, error) {
	if args.NumUnits > 1 && len(args.Placement) == 0 && args.ToMachineSpec != "" {
		return nil, fmt.Errorf("cannot use --num-units with --to")
	}
	settings, err := args.Charm.Config().ValidateSettings(args.ConfigSettings)
	if err != nil {
		return nil, err
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 || args.ToMachineSpec != "" {
			return nil, fmt.Errorf("subordinate service must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return nil, fmt.Errorf("subordinate service must be deployed without constraints")
		}
	}
	if args.ServiceOwner == "" {
		env, err := st.Environment()
		if err != nil {
			return nil, errors.Trace(err)
		}
		args.ServiceOwner = env.Owner().String()
	}
	// TODO(fwereade): transactional State.AddService including settings, constraints
	// (minimumUnitCount, initialMachineIds?).

	if len(args.Networks) > 0 || args.Constraints.HaveNetworks() {
		return nil, fmt.Errorf("use of --networks is deprecated. Please use spaces")
	}

	// TODO(dimitern): In a follow-up drop Networks and use spaces
	// constraints for this when possible.
	service, err := st.AddService(
		args.ServiceName,
		args.ServiceOwner,
		args.Charm,
		args.Networks,
		stateStorageConstraints(args.Storage),
	)
	if err != nil {
		return nil, err
	}
	if len(settings) > 0 {
		if err := service.UpdateConfigSettings(settings); err != nil {
			return nil, err
		}
	}
	if args.Charm.Meta().Subordinate {
		return service, nil
	}
	if !constraints.IsEmpty(&args.Constraints) {
		if err := service.SetConstraints(args.Constraints); err != nil {
			return nil, err
		}
	}
	if args.NumUnits > 0 {
		var err error
		// We either have a machine spec or a placement directive.
		// Placement directives take precedence.
		if len(args.Placement) > 0 || args.ToMachineSpec == "" {
			_, err = AddUnitsWithPlacement(st, service, args.NumUnits, args.Placement)
		} else {
			_, err = AddUnits(st, service, args.NumUnits, args.ToMachineSpec)
		}
		if err != nil {
			return nil, err
		}
	}
	return service, nil
}

func addMachineForUnit(st *state.State, unit *state.Unit, placement *instance.Placement, networks []string) (*state.Machine, error) {
	unitCons, err := unit.Constraints()
	if err != nil {
		return nil, err
	}
	var containerType instance.ContainerType
	var mid, placementDirective string
	// Extract container type and parent from container placement directives.
	if containerType, err = instance.ParseContainerType(placement.Scope); err == nil {
		mid = placement.Directive
	} else {
		switch placement.Scope {
		case st.EnvironUUID():
			placementDirective = placement.Directive
		case instance.MachineScope:
			mid = placement.Directive
		default:
			return nil, errors.Errorf("invalid environment UUID %q", placement.Scope)
		}
	}

	// Create any new machine marked as dirty so that
	// nothing else will grab it before we assign the unit to it.

	// If a container is to be used, create it.
	if containerType != "" {
		template := state.MachineTemplate{
			Series:            unit.Series(),
			Jobs:              []state.MachineJob{state.JobHostUnits},
			Dirty:             true,
			Constraints:       *unitCons,
			RequestedNetworks: networks,
		}
		return st.AddMachineInsideMachine(template, mid, containerType)
	}
	// If a placement directive is to be used, do that here.
	if placementDirective != "" {
		template := state.MachineTemplate{
			Series:            unit.Series(),
			Jobs:              []state.MachineJob{state.JobHostUnits},
			Dirty:             true,
			Constraints:       *unitCons,
			RequestedNetworks: networks,
			Placement:         placementDirective,
		}
		return st.AddOneMachine(template)
	}

	// Otherwise use an existing machine.
	return st.Machine(mid)
}

// AddUnits starts n units of the given service and allocates machines
// to them as necessary.
func AddUnits(st *state.State, svc *state.Service, n int, machineIdSpec string) ([]*state.Unit, error) {
	if machineIdSpec != "" && n != 1 {
		return nil, errors.Errorf("cannot add multiple units of service %q to a single machine", svc.Name())
	}
	var placement []*instance.Placement
	if machineIdSpec != "" {
		mid := machineIdSpec
		scope := instance.MachineScope
		var containerType instance.ContainerType
		specParts := strings.SplitN(machineIdSpec, ":", 2)
		if len(specParts) > 1 {
			firstPart := specParts[0]
			var err error
			if containerType, err = instance.ParseContainerType(firstPart); err == nil {
				mid = specParts[1]
				scope = string(containerType)
			}
		}
		if !names.IsValidMachine(mid) {
			return nil, fmt.Errorf("invalid force machine id %q", mid)
		}
		placement = []*instance.Placement{
			{
				Scope:     scope,
				Directive: mid,
			},
		}
	}
	return AddUnitsWithPlacement(st, svc, n, placement)
}

// AddUnitsWithPlacement starts n units of the given service using the specified placement
// directives to allocate the machines.
func AddUnitsWithPlacement(st *state.State, svc *state.Service, n int, placement []*instance.Placement) ([]*state.Unit, error) {
	units := make([]*state.Unit, n)
	// Hard code for now till we implement a different approach.
	policy := state.AssignCleanEmpty
	// All units should have the same networks as the service.
	networks, err := svc.Networks()
	if err != nil {
		return nil, errors.Errorf("cannot get service %q networks", svc.Name())
	}
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := svc.AddUnit()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot add unit %d/%d to service %q", i+1, n, svc.Name())
		}
		// Are there still placement directives to use?
		if i > len(placement)-1 {
			if err := st.AssignUnit(unit, policy); err != nil {
				return nil, errors.Trace(err)
			}
			units[i] = unit
			continue
		}
		m, err := addMachineForUnit(st, unit, placement[i], networks)
		if err != nil {
			return nil, errors.Annotatef(err, "adding new machine to host unit %q", unit.Name())
		}
		if err = unit.AssignToMachine(m); err != nil {
			return nil, errors.Trace(err)
		}
		units[i] = unit
	}
	return units, nil
}

func stateStorageConstraints(cons map[string]storage.Constraints) map[string]state.StorageConstraints {
	result := make(map[string]state.StorageConstraints)
	for name, cons := range cons {
		result[name] = state.StorageConstraints{
			Pool:  cons.Pool,
			Size:  cons.Size,
			Count: cons.Count,
		}
	}
	return result
}
