// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	coreassumes "github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	coreunit "github.com/juju/juju/core/unit"
	applicationservice "github.com/juju/juju/domain/application/service"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state"
)

// ModelService provides access to the model state.
type ModelService interface {
	// GetSupportedFeatures returns the set of features that the model makes
	// available for charms to use.
	GetSupportedFeatures(ctx context.Context) (coreassumes.FeatureSet, error)
}

// DeployApplicationParams contains the arguments required to deploy the referenced
// charm.
type DeployApplicationParams struct {
	ApplicationName   string
	Charm             Charm
	CharmOrigin       corecharm.Origin
	ApplicationConfig *config.Config
	CharmConfig       charm.Settings
	Constraints       constraints.Value
	NumUnits          int
	// Placement is a list of placement directives which may be used
	// instead of a machine spec.
	Placement        []*instance.Placement
	Storage          map[string]storage.Directive
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
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (Application, error)

	// ReadSequence is a stop gap to allow the next unit number to be read from mongo
	// so that correctly matching units can be written to dqlite.
	ReadSequence(name string) (int, error)
}

type UnitAdder interface {
	AddUnit(state.AddUnitParams) (Unit, error)
}

// DeployApplication takes a charm and various parameters and deploys it.
func DeployApplication(
	ctx context.Context, st ApplicationDeployer, model Model,
	modelInfo coremodel.ReadOnlyModel,
	applicationService ApplicationService,
	store objectstore.ObjectStore,
	args DeployApplicationParams,
	logger corelogger.Logger,
) (Application, error) {
	charmConfig, err := args.Charm.Config().ValidateSettings(args.CharmConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if args.Charm.Meta().Name == bootstrap.ControllerCharmName {
		return nil, errors.NotSupportedf("manual deploy of the controller charm")
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 {
			return nil, fmt.Errorf("subordinate application must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return nil, fmt.Errorf("subordinate application must be deployed without constraints")
		}
	}

	// Enforce "assumes" requirements.
	if err := assertCharmAssumptions(ctx, applicationService, args.Charm.Meta().Assumes); err != nil {
		if !errors.Is(err, errors.NotSupported) || !args.Force {
			return nil, errors.Trace(err)
		}

		logger.Warningf(context.TODO(), "proceeding with deployment of application %q even though the charm feature requirements could not be met as --force was specified", args.ApplicationName)
	}

	if modelInfo.Type == coremodel.CAAS {
		if charm.MetaFormat(args.Charm) == charm.FormatV1 {
			return nil, errors.NotSupportedf("deploying format v1 charm %q", args.ApplicationName)
		}
	}

	// TODO(fwereade): transactional State.AddApplication including settings, constraints
	// (minimumUnitCount, initialMachineIds?).

	origin, err := StateCharmOrigin(args.CharmOrigin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	asa := state.AddApplicationArgs{
		Name:              args.ApplicationName,
		Charm:             args.Charm,
		CharmOrigin:       origin,
		Storage:           stateStorageDirectives(args.Storage),
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

	// TODO(dqlite) - remove mongo AddApplication call.
	// To ensure dqlite unit names match those created in mongo, grab the next unit
	// sequence number before writing the mongo units.
	nextUnitNum, err := st.ReadSequence(args.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitArgs := make([]applicationservice.AddUnitArg, args.NumUnits)
	for i := 0; i < args.NumUnits; i++ {
		unitName, err := coreunit.NewNameFromParts(args.ApplicationName, nextUnitNum+i)
		if err != nil {
			return nil, errors.Trace(err)
		}
		unitArgs[i].UnitName = unitName
	}
	app, err := st.AddApplication(asa, store)

	// Dual write storage directives to dqlite.
	if err == nil {
		chURL, err := charm.ParseURL(args.Charm.URL())
		if err != nil {
			return nil, errors.Trace(err)
		}

		_, err = applicationService.CreateApplication(ctx, args.ApplicationName, args.Charm, args.CharmOrigin, applicationservice.AddApplicationArgs{
			ReferenceName: chURL.Name,
			Storage:       args.Storage,
		}, unitArgs...)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return app, errors.Trace(err)
}

// addUnits starts n units of the given application using the specified placement
// directives to allocate the machines.
func (api *APIBase) addUnits(
	ctx context.Context,
	unitAdder UnitAdder,
	appName string,
	n int,
	placement []*instance.Placement,
	attachStorage []names.StorageTag,
	assignUnits bool,
) ([]Unit, error) {
	units := make([]Unit, n)
	policy := state.AssignNew

	allSpaces, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, err
	}

	machineToUnitMap := make(map[string][]coreunit.Name)

	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := unitAdder.AddUnit(state.AddUnitParams{
			AttachStorage: attachStorage,
		})
		if err != nil {
			return nil, internalerrors.Errorf("adding unit %d/%d to application %q: %w", i+1, n, appName, err)
		}
		unitName, err := coreunit.NewName(unit.Name())
		if err != nil {
			return nil, internalerrors.Errorf("parsing unit name %q: %w", unit.Name(), err)
		}
		if err := api.applicationService.AddUnits(ctx, appName, applicationservice.AddUnitArg{UnitName: unitName}); err != nil {
			return nil, internalerrors.Errorf("adding unit %q to application %q: %w", unitName, appName, err)
		}
		units[i] = unit
		if !assignUnits {
			continue
		}

		// Are there still placement directives to use?
		if i > len(placement)-1 {
			if err := unit.AssignWithPolicy(policy); err != nil {
				return nil, internalerrors.Errorf("acquiring machine for policy %q to host unit %q: %w", policy, unitName, err)
			}
		} else {
			if err := unit.AssignWithPlacement(placement[i], allSpaces); err != nil {
				return nil, internalerrors.Errorf("acquiring machine for placement %q to host unit %q: %w", placement[i], unitName, err)
			}
		}

		// Get assigned machine and ensure it exists in dqlite.
		id, err := unit.AssignedMachineId()
		if err != nil {
			return nil, internalerrors.Errorf("getting assigned machine for unit %q: %w", unitName, err)
		}
		if err := saveMachineInfo(ctx, api.machineService, id); err != nil {
			return nil, internalerrors.Errorf("saving assigned machine %q for unit %q: %w", id, unitName, err)
		}
		machineToUnitMap[id] = append(machineToUnitMap[id], unitName)
	}

	// Assign units to machines via net nodes.
	if err := api.stubService.AssignUnitsToMachines(ctx, machineToUnitMap); err != nil {
		return nil, internalerrors.Errorf("assigning units to machines: %w", err)
	}

	return units, nil
}

func saveMachineInfo(ctx context.Context, machineService MachineService, machineName string) error {
	// This is temporary - just insert the machine id and all the parent ones.
	for machineName != "" {
		_, err := machineService.CreateMachine(ctx, machine.Name(machineName))
		// The machine might already exist e.g. if we are adding a subordinate
		// unit to an already existing machine. In this case, just continue
		// without error.
		if err != nil && !errors.Is(err, machineerrors.MachineAlreadyExists) {
			return errors.Annotatef(err, "saving info for machine %q", machineName)
		}
		parent := names.NewMachineTag(machineName).Parent()
		if parent == nil {
			break
		}
		machineName = parent.Id()
	}
	return nil
}

func stateStorageDirectives(cons map[string]storage.Directive) map[string]state.StorageConstraints {
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

// StateCharmOrigin returns a state layer CharmOrigin given a core Origin.
func StateCharmOrigin(origin corecharm.Origin) (*state.CharmOrigin, error) {
	var ch *state.Channel
	if c := origin.Channel; c != nil {
		normalizedC := c.Normalize()
		ch = &state.Channel{
			Track:  normalizedC.Track,
			Risk:   string(normalizedC.Risk),
			Branch: normalizedC.Branch,
		}
	}
	return &state.CharmOrigin{
		Type:     origin.Type,
		Source:   string(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel:  ch,
		Platform: &state.Platform{
			Architecture: origin.Platform.Architecture,
			OS:           origin.Platform.OS,
			Channel:      origin.Platform.Channel,
		},
	}, nil
}

func assertCharmAssumptions(
	ctx context.Context,
	applicationService ApplicationService,
	assumesExprTree *assumes.ExpressionTree,
) error {
	if assumesExprTree == nil {
		return nil
	}

	featureSet, err := applicationService.GetSupportedFeatures(ctx)
	if err != nil {
		return errors.Annotate(err, "querying feature set supported by the model")
	}

	if err = featureSet.Satisfies(assumesExprTree); err != nil {
		return errors.NewNotSupported(err, "")
	}

	return nil
}
