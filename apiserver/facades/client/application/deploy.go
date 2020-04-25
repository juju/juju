// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/charm/v7"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// DeployApplicationParams contains the arguments required to deploy the referenced
// charm.
type DeployApplicationParams struct {
	ApplicationName   string
	Series            string
	Charm             *state.Charm
	Channel           csparams.Channel
	ApplicationConfig *application.Config
	CharmConfig       charm.Settings
	Constraints       constraints.Value
	NumUnits          int
	// Placement is a list of placement directives which may be used
	// instead of a machine spec.
	Placement        []*instance.Placement
	Storage          map[string]storage.Constraints
	Devices          map[string]devices.Constraints
	AttachStorage    []names.StorageTag
	EndpointBindings map[string]string
	// Resources is a map of resource name to IDs of pending resources.
	Resources map[string]string
}

type ApplicationDeployer interface {
	AddApplication(state.AddApplicationArgs) (Application, error)
}

type UnitAdder interface {
	AddUnit(state.AddUnitParams) (Unit, error)
}

// DeployApplication takes a charm and various parameters and deploys it.
func DeployApplication(st ApplicationDeployer, args DeployApplicationParams) (Application, error) {
	charmConfig, err := args.Charm.Config().ValidateSettings(args.CharmConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 {
			return nil, fmt.Errorf("subordinate application must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return nil, fmt.Errorf("subordinate application must be deployed without constraints")
		}
	}
	// TODO(fwereade): transactional State.AddApplication including settings, constraints
	// (minimumUnitCount, initialMachineIds?).

	asa := state.AddApplicationArgs{
		Name:              args.ApplicationName,
		Series:            args.Series,
		Charm:             args.Charm,
		Channel:           args.Channel,
		Storage:           stateStorageConstraints(args.Storage),
		Devices:           stateDeviceConstraints(args.Devices),
		AttachStorage:     args.AttachStorage,
		ApplicationConfig: args.ApplicationConfig,
		CharmConfig:       charmConfig,
		NumUnits:          args.NumUnits,
		Placement:         args.Placement,
		Resources:         args.Resources,
		EndpointBindings:  args.EndpointBindings,
	}

	if !args.Charm.Meta().Subordinate {
		asa.Constraints = args.Constraints
	}
	return st.AddApplication(asa)
}

func quoteStrings(vals []string) string {
	out := make([]string, len(vals))
	for i, val := range vals {
		out[i] = strconv.Quote(val)
	}
	return strings.Join(out, ", ")
}

// addUnits starts n units of the given application using the specified placement
// directives to allocate the machines.
func addUnits(
	unitAdder UnitAdder,
	appName string,
	n int,
	placement []*instance.Placement,
	attachStorage []names.StorageTag,
	assignUnits bool,
) ([]Unit, error) {
	units := make([]Unit, n)
	// Hard code for now till we implement a different approach.
	policy := state.AssignCleanEmpty
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := unitAdder.AddUnit(state.AddUnitParams{
			AttachStorage: attachStorage,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "cannot add unit %d/%d to application %q", i+1, n, appName)
		}
		units[i] = unit
		if !assignUnits {
			continue
		}

		// Are there still placement directives to use?
		if i > len(placement)-1 {
			if err := unit.AssignWithPolicy(policy); err != nil {
				return nil, errors.Trace(err)
			}
			continue
		}
		if err := unit.AssignWithPlacement(placement[i]); err != nil {
			return nil, errors.Annotatef(err, "acquiring machine to host unit %q", unit.UnitTag().Id())
		}
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

func stateDeviceConstraints(cons map[string]devices.Constraints) map[string]state.DeviceConstraints {
	result := make(map[string]state.DeviceConstraints)
	for name, cons := range cons {
		result[name] = state.DeviceConstraints{
			Type:       state.DeviceType(cons.Type),
			Count:      cons.Count,
			Attributes: cons.Attributes,
		}
	}
	return result
}
