// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/assumes"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
)

var (
	// Overridden by tests.
	supportedFeaturesGetter = stateenvirons.SupportedFeatures
)

// DeployApplicationParams contains the arguments required to deploy the referenced
// charm.
type DeployApplicationParams struct {
	ApplicationName   string
	Series            string
	Charm             *state.Charm
	CharmOrigin       corecharm.Origin
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

	// If set to true, any charm-specific requirements ("assumes" section)
	// will be ignored.
	Force bool
}

type ApplicationDeployer interface {
	AddApplication(state.AddApplicationArgs) (Application, error)
	ControllerConfig() (controller.Config, error)
}

type UnitAdder interface {
	AddUnit(state.AddUnitParams) (Unit, error)
}

// DeployApplication takes a charm and various parameters and deploys it.
func DeployApplication(st ApplicationDeployer, model Model, args DeployApplicationParams) (Application, error) {
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

	// Enforce "assumes" requirements if the feature flag is enabled.
	if err := assertCharmAssumptions(args.Charm.Meta().Assumes, model, st.ControllerConfig); err != nil {
		if !errors.IsNotSupported(err) || !args.Force {
			return nil, errors.Trace(err)
		}

		logger.Warningf("proceeding with deployment of application %q even though the charm feature requirements could not be met as --force was specified", args.ApplicationName)
	}

	// TODO(fwereade): transactional State.AddApplication including settings, constraints
	// (minimumUnitCount, initialMachineIds?).

	asa := state.AddApplicationArgs{
		Name:              args.ApplicationName,
		Series:            args.Series,
		Charm:             args.Charm,
		CharmOrigin:       stateCharmOrigin(args.CharmOrigin),
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

func stateCharmOrigin(origin corecharm.Origin) *state.CharmOrigin {
	var ch *state.Channel
	if c := origin.Channel; c != nil {
		normalizedC := c.Normalize()
		ch = &state.Channel{
			Track:  normalizedC.Track,
			Risk:   string(normalizedC.Risk),
			Branch: normalizedC.Branch,
		}
	}
	stateOrigin := &state.CharmOrigin{
		Type:     origin.Type,
		Source:   string(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel:  ch,
		Platform: &state.Platform{
			Architecture: origin.Platform.Architecture,
			OS:           origin.Platform.OS,
			Series:       origin.Platform.Series,
		},
	}
	return stateOrigin
}

func assertCharmAssumptions(assumesExprTree *assumes.ExpressionTree, model Model, ctrlCfgGetter func() (controller.Config, error)) error {
	if assumesExprTree == nil {
		return nil
	}

	ctrlCfg, err := ctrlCfgGetter()
	if err != nil {
		return errors.Trace(err)
	}

	if !ctrlCfg.Features().Contains(feature.CharmAssumes) {
		return nil
	}

	featureSet, err := supportedFeaturesGetter(model, environs.New)
	if err != nil {
		return errors.Annotate(err, "querying feature set supported by the model")
	}

	if err = featureSet.Satisfies(assumesExprTree); err != nil {
		return errors.NewNotSupported(err, "")
	}

	return nil
}
