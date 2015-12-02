// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// DeployServiceParams contains the arguments required to deploy the referenced
// charm.
type DeployServiceParams struct {
	ServiceName    string
	Series         string
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

type ServiceDeployer interface {
	Environment() (*state.Environment, error)
	AddService(state.AddServiceArgs) (*state.Service, error)
}

// DeployService takes a charm and various parameters and deploys it.
func DeployService(st ServiceDeployer, args DeployServiceParams) (*state.Service, error) {
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

	if len(args.Placement) == 0 {
		args.Placement, err = makePlacement(args.ToMachineSpec)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	asa := state.AddServiceArgs{
		Name:      args.ServiceName,
		Series:    args.Series,
		Owner:     args.ServiceOwner,
		Charm:     args.Charm,
		Networks:  args.Networks,
		Storage:   stateStorageConstraints(args.Storage),
		Settings:  settings,
		NumUnits:  args.NumUnits,
		Placement: args.Placement,
	}

	if !args.Charm.Meta().Subordinate {
		asa.Constraints = args.Constraints
	}

	// TODO(dimitern): In a follow-up drop Networks and use spaces
	// constraints for this when possible.
	return st.AddService(asa)
}

// AddUnits starts n units of the given service and allocates machines
// to them as necessary.
func AddUnits(st *state.State, svc *state.Service, n int, machineIdSpec string) ([]*state.Unit, error) {
	if machineIdSpec != "" && n != 1 {
		return nil, errors.Errorf("cannot add multiple units of service %q to a single machine", svc.Name())
	}
	placement, err := makePlacement(machineIdSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return AddUnitsWithPlacement(st, svc, n, placement)
}

// makePlacement makes a placement directive for the given machineIdSpec.
func makePlacement(machineIdSpec string) ([]*instance.Placement, error) {
	if machineIdSpec == "" {
		return nil, nil
	}
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
		return nil, errors.Errorf("invalid force machine id %q", mid)
	}
	return []*instance.Placement{
		{
			Scope:     scope,
			Directive: mid,
		},
	}, nil
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
		if err := st.AssignUnitWithPlacement(unit, placement[i], networks); err != nil {
			return nil, errors.Annotatef(err, "adding new machine to host unit %q", unit.Name())
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
