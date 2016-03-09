// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"

	"github.com/juju/errors"
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
	// Placement is a list of placement directives which may be used
	// instead of a machine spec.
	Placement []*instance.Placement
	// Networks holds a list of networks to required to start on boot.
	// TODO(dimitern): Drop this in a follow-up in favor of constraints.
	Networks         []string
	Storage          map[string]storage.Constraints
	EndpointBindings map[string]string
	// Resources is a map of resource name to IDs of pending resources.
	Resources map[string]string
}

type ServiceDeployer interface {
	Model() (*state.Model, error)
	AddService(state.AddServiceArgs) (*state.Service, error)
}

// DeployService takes a charm and various parameters and deploys it.
func DeployService(st ServiceDeployer, args DeployServiceParams) (*state.Service, error) {
	settings, err := args.Charm.Config().ValidateSettings(args.ConfigSettings)
	if err != nil {
		return nil, err
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 {
			return nil, fmt.Errorf("subordinate service must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return nil, fmt.Errorf("subordinate service must be deployed without constraints")
		}
	}
	if args.ServiceOwner == "" {
		env, err := st.Model()
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

	effectiveBindings, err := getEffectiveBindingsForCharmMeta(args.Charm.Meta(), args.EndpointBindings)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine effective service endpoint bindings")
	}

	asa := state.AddServiceArgs{
		Name:             args.ServiceName,
		Series:           args.Series,
		Owner:            args.ServiceOwner,
		Charm:            args.Charm,
		Networks:         args.Networks,
		Storage:          stateStorageConstraints(args.Storage),
		Settings:         settings,
		NumUnits:         args.NumUnits,
		Placement:        args.Placement,
		Resources:        args.Resources,
		EndpointBindings: effectiveBindings,
	}

	if !args.Charm.Meta().Subordinate {
		asa.Constraints = args.Constraints
	}

	// TODO(dimitern): In a follow-up drop Networks and use spaces
	// constraints for this when possible.
	return st.AddService(asa)
}

func getEffectiveBindingsForCharmMeta(charmMeta *charm.Meta, givenBindings map[string]string) (map[string]string, error) {
	combinedEndpoints, err := state.CombinedCharmRelations(charmMeta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if givenBindings == nil {
		givenBindings = make(map[string]string, len(combinedEndpoints))
	}

	// Get the service-level default binding for all unspecified endpoint, if
	// set, otherwise use the empty default.
	serviceDefaultSpace, _ := givenBindings[""]

	effectiveBindings := make(map[string]string, len(combinedEndpoints))
	for endpoint, _ := range combinedEndpoints {
		if givenSpace, isGiven := givenBindings[endpoint]; isGiven {
			effectiveBindings[endpoint] = givenSpace
		} else {
			effectiveBindings[endpoint] = serviceDefaultSpace
		}
	}
	return effectiveBindings, nil
}

// AddUnits starts n units of the given service using the specified placement
// directives to allocate the machines.
func AddUnits(st *state.State, svc *state.Service, n int, placement []*instance.Placement) ([]*state.Unit, error) {
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
