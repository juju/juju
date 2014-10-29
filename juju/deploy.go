// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
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
	// Networks holds a list of networks to required to start on boot.
	Networks []string
}

// DeployService takes a charm and various parameters and deploys it.
func DeployService(st *state.State, args DeployServiceParams) (*state.Service, error) {
	if args.NumUnits > 1 && args.ToMachineSpec != "" {
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
		conf, err := st.EnvironConfig()
		if err != nil {
			return nil, err
		}
		env, err := environs.New(conf)
		if err != nil {
			return nil, err
		}
		if !env.SupportNetworks() {
			return nil, fmt.Errorf("cannot deploy with networks: not suppored by the environment")
		}
	}
	service, err := st.AddService(
		args.ServiceName,
		args.ServiceOwner,
		args.Charm,
		args.Networks,
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
		if _, err := AddUnits(st, service, args.NumUnits, args.ToMachineSpec); err != nil {
			return nil, err
		}
	}
	return service, nil
}

// AddUnits starts n units of the given service and allocates machines
// to them as necessary.
func AddUnits(st *state.State, svc *state.Service, n int, machineIdSpec string) ([]*state.Unit, error) {
	units := make([]*state.Unit, n)
	// Hard code for now till we implement a different approach.
	policy := state.AssignCleanEmpty
	// All units should have the same networks as the service.
	networks, err := svc.Networks()
	if err != nil {
		return nil, fmt.Errorf("cannot get service %q networks: %v", svc.Name(), err)
	}
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := svc.AddUnit()
		if err != nil {
			return nil, fmt.Errorf("cannot add unit %d/%d to service %q: %v", i+1, n, svc.Name(), err)
		}
		if machineIdSpec != "" {
			if n != 1 {
				return nil, fmt.Errorf("cannot add multiple units of service %q to a single machine", svc.Name())
			}
			// machineIdSpec may be an existing machine or container, eg 3/lxc/2
			// or a new container on a machine, eg lxc:3
			mid := machineIdSpec
			var containerType instance.ContainerType
			specParts := strings.SplitN(machineIdSpec, ":", 2)
			if len(specParts) > 1 {
				firstPart := specParts[0]
				var err error
				if containerType, err = instance.ParseContainerType(firstPart); err == nil {
					mid = specParts[1]
				} else {
					mid = machineIdSpec
				}
			}
			if !names.IsValidMachine(mid) {
				return nil, fmt.Errorf("invalid force machine id %q", mid)
			}
			var unitCons *constraints.Value
			unitCons, err = unit.Constraints()
			if err != nil {
				return nil, err
			}

			var err error
			var m *state.Machine
			// If a container is to be used, create it.
			if containerType != "" {
				// Create the new machine marked as dirty so that
				// nothing else will grab it before we assign the unit to it.
				template := state.MachineTemplate{
					Series:            unit.Series(),
					Jobs:              []state.MachineJob{state.JobHostUnits},
					Dirty:             true,
					Constraints:       *unitCons,
					RequestedNetworks: networks,
				}
				m, err = st.AddMachineInsideMachine(template, mid, containerType)
			} else {
				m, err = st.Machine(mid)
			}
			if err != nil {
				return nil, fmt.Errorf("cannot assign unit %q to machine: %v", unit.Name(), err)
			}
			err = unit.AssignToMachine(m)

			if err != nil {
				return nil, err
			}
		} else if err := st.AssignUnit(unit, policy); err != nil {
			return nil, err
		}
		units[i] = unit
	}
	return units, nil
}
