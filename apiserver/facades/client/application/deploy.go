// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/internal/charms"
	coreapplication "github.com/juju/juju/core/application"
	coreassumes "github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
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
	EndpointBindings map[string]network.SpaceName
	// Resources is a map of resource name to IDs of pending resources.
	Resources map[string]string

	// If set to true, any charm-specific requirements ("assumes" section)
	// will be ignored.
	Force bool
}

type ApplicationDeployer interface {
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (Application, error)
}

type UnitAdder interface {
	AddUnit(state.AddUnitParams) (Unit, error)
}

// DeployApplication takes a charm and various parameters and deploys it.
func DeployApplication(
	ctx context.Context, st ApplicationDeployer,
	modelType coremodel.ModelType,
	applicationService ApplicationService,
	store objectstore.ObjectStore,
	args DeployApplicationParams,
	logger corelogger.Logger,
	clock clock.Clock,
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

		logger.Warningf(ctx, "proceeding with deployment of application %q even though the charm feature requirements could not be met as --force was specified", args.ApplicationName)
	}

	if modelType == coremodel.CAAS {
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
		CharmURL:          args.Charm.URL(),
		CharmOrigin:       origin,
		Storage:           stateStorageDirectives(args.Storage),
		AttachStorage:     args.AttachStorage,
		ApplicationConfig: args.ApplicationConfig,
		CharmConfig:       charmConfig,
		NumUnits:          args.NumUnits,
		Placement:         args.Placement,
		Resources:         args.Resources,
	}

	if !args.Charm.Meta().Subordinate {
		asa.Constraints = args.Constraints
	}

	app, err := st.AddApplication(asa, store)

	// Dual write storage directives to dqlite.
	if err == nil {
		chURL, err := charm.ParseURL(args.Charm.URL())
		if err != nil {
			return nil, errors.Trace(err)
		}

		var downloadInfo *applicationcharm.DownloadInfo
		if args.CharmOrigin.Source == corecharm.CharmHub {
			locator, err := charms.CharmLocatorFromURL(args.Charm.URL())
			if err != nil {
				return nil, errors.Trace(err)
			}
			downloadInfo, err = applicationService.GetCharmDownloadInfo(ctx, locator)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}

		pendingResources, err := transformToPendingResources(args.Resources)
		if err != nil {
			return nil, errors.Trace(err)
		}

		attrs := args.ApplicationConfig.Attributes()
		trust := attrs.GetBool(coreapplication.TrustConfigOptionName, false)

		applicationArg := applicationservice.AddApplicationArgs{
			ReferenceName:    chURL.Name,
			Storage:          args.Storage,
			DownloadInfo:     downloadInfo,
			PendingResources: pendingResources,
			EndpointBindings: args.EndpointBindings,
			Devices:          args.Devices,
			ApplicationStatus: &status.StatusInfo{
				Status: status.Unset,
				Since:  ptr(clock.Now()),
			},
			ApplicationConfig: config.ConfigAttributes(charmConfig),
			ApplicationSettings: application.ApplicationSettings{
				Trust: trust,
			},
		}
		if modelType == coremodel.CAAS {
			unitArgs, err := makeCAASUnitArgs(args)
			if err != nil {
				return nil, errors.Trace(err)
			}

			_, err = applicationService.CreateCAASApplication(
				ctx,
				args.ApplicationName,
				args.Charm,
				args.CharmOrigin,
				applicationArg,
				unitArgs...,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else {
			unitArgs, err := makeIAASUnitArgs(args)
			if err != nil {
				return nil, errors.Trace(err)
			}

			_, err = applicationService.CreateIAASApplication(
				ctx,
				args.ApplicationName,
				args.Charm,
				args.CharmOrigin,
				applicationArg,
				unitArgs...,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}
	return app, errors.Trace(err)
}

func makeIAASUnitArgs(args DeployApplicationParams) ([]applicationservice.AddIAASUnitArg, error) {
	unitArgs := make([]applicationservice.AddIAASUnitArg, args.NumUnits)
	for i := range args.NumUnits {
		var unitPlacement *instance.Placement
		if i < len(args.Placement) {
			unitPlacement = args.Placement[i]
		}
		unitArgs[i] = applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				Placement: unitPlacement,
			},
		}
	}

	return unitArgs, nil
}

func makeCAASUnitArgs(args DeployApplicationParams) ([]applicationservice.AddUnitArg, error) {
	unitArgs := make([]applicationservice.AddUnitArg, args.NumUnits)
	for i := range args.NumUnits {
		var unitPlacement *instance.Placement
		if i < len(args.Placement) {
			unitPlacement = args.Placement[i]
		}
		unitArgs[i] = applicationservice.AddUnitArg{
			Placement: unitPlacement,
		}
	}

	return unitArgs, nil
}

func transformBindings(endpointBindings map[string]string) map[string]network.SpaceName {
	bindings := make(map[string]network.SpaceName)
	for endpoint, space := range endpointBindings {
		bindings[endpoint] = network.SpaceName(space)
	}
	return bindings
}

func transformToPendingResources(argResources map[string]string) ([]coreresource.UUID, error) {
	var pendingResources []coreresource.UUID
	for _, res := range argResources {
		resUUID, err := coreresource.ParseUUID(res)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pendingResources = append(pendingResources, resUUID)
	}
	return pendingResources, nil
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
	charmMeta *charm.Meta,
) ([]coreunit.Name, error) {
	units := make([]coreunit.Name, 0, n)

	allSpaces, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, err
	}

	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := unitAdder.AddUnit(state.AddUnitParams{
			AttachStorage: attachStorage,
			CharmMeta:     charmMeta,
		})
		if err != nil {
			return nil, internalerrors.Errorf("adding unit %d/%d to application %q: %w", i+1, n, appName, err)
		}

		var unitPlacement *instance.Placement
		if i < len(placement) {
			unitPlacement = placement[i]
		}

		unitArg := applicationservice.AddUnitArg{
			Placement: unitPlacement,
		}

		var unitNames []coreunit.Name

		if api.modelType == coremodel.CAAS {
			unitNames, err = api.applicationService.AddCAASUnits(ctx, appName, unitArg)
		} else {
			unitNames, err = api.applicationService.AddIAASUnits(ctx, appName, applicationservice.AddIAASUnitArg{
				AddUnitArg: unitArg,
			})
		}
		if err != nil {
			return nil, internalerrors.Errorf("adding unit to application %q: %w", appName, err)
		}
		units = append(units, unitNames...)
		if !assignUnits {
			continue
		}

		// Are there still placement directives to use?
		if i > len(placement)-1 {
			if err := unit.AssignUnit(); err != nil {
				return nil, internalerrors.Errorf("acquiring new machine to host unit: %w", err)
			}
		} else {
			if err := unit.AssignWithPlacement(placement[i], allSpaces); err != nil {
				return nil, internalerrors.Errorf("acquiring machine for placement %q to host unit: %w", placement[i], err)
			}
		}
	}

	return units, nil
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
