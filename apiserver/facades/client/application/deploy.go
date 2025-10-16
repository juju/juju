// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	coreassumes "github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	coreerrors "github.com/juju/juju/core/errors"
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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
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
	Charm             *domainCharm
	CharmOrigin       corecharm.Origin
	Trust             bool
	ApplicationConfig charm.Config
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

// handleApplicationDomainError is a first low pass effort to start handling
// some of the errors that will come out of the application domain. If a handler
// does not exist then the original error will be returned.
func handleApplicationDomainError(err error) error {
	// Check to see if the user has creeped over or under a charm storage count
	// limit.
	limitErr, is := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
	if is && limitErr.Requested < limitErr.Minimum {
		return errors.Errorf(
			"storage directive %q request count %d insufficient for the charms minimum count of %d",
			limitErr.StorageName, limitErr.Requested, limitErr.Minimum,
		).Add(coreerrors.NotValid)
	} else if is && limitErr.Maximum != nil && limitErr.Requested > *limitErr.Maximum {
		return errors.Errorf(
			"storage directive %q request count %d exceeds the charms maximum count of %d",
			limitErr.StorageName, limitErr.Requested, *limitErr.Maximum,
		).Add(coreerrors.NotValid)
	}

	return err
}

// DeployApplication takes a charm and various parameters and deploys it.
func DeployApplication(
	ctx context.Context,
	modelType coremodel.ModelType,
	applicationService ApplicationService,
	storageService StorageService,
	store objectstore.ObjectStore,
	args DeployApplicationParams,
	logger corelogger.Logger,
	clock clock.Clock,
) error {
	if args.Charm.Meta().Name == bootstrap.ControllerCharmName {
		return errors.New("manual deploy of the controller charm not supported").
			Add(coreerrors.NotSupported)
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 {
			return errors.New("subordinate application must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return errors.Errorf("subordinate application must be deployed without constraints, not %q", args.Constraints)
		}
	}

	// Enforce "assumes" requirements.
	if err := assertCharmAssumptions(ctx, applicationService, args.Charm.Meta().Assumes); err != nil {
		if !errors.Is(err, coreerrors.NotSupported) || !args.Force {
			return errors.Capture(err)
		}

		logger.Warningf(ctx, "proceeding with deployment of application %q even though the charm feature requirements could not be met as --force was specified", args.ApplicationName)
	}

	if modelType == coremodel.CAAS {
		if charm.MetaFormat(args.Charm) == charm.FormatV1 {
			return errors.Errorf(
				"deploying format v1 charm %q not supported",
				args.ApplicationName,
			).Add(coreerrors.NotSupported)
		}
	}

	var downloadInfo *applicationcharm.DownloadInfo
	if args.CharmOrigin.Source == corecharm.CharmHub {
		var err error
		downloadInfo, err = applicationService.GetCharmDownloadInfo(ctx, args.Charm.locator)
		if err != nil {
			return errors.Capture(err)
		}
	}

	pendingResources, err := transformToPendingResources(args.Resources)
	if err != nil {
		return errors.Capture(err)
	}

	sdo, err := storageDirectives(ctx, storageService, args.Storage)
	if err != nil {
		return errors.Capture(err)
	}

	applicationArg := applicationservice.AddApplicationArgs{
		ReferenceName:    args.Charm.locator.Name,
		DownloadInfo:     downloadInfo,
		PendingResources: pendingResources,
		EndpointBindings: args.EndpointBindings,
		Devices:          args.Devices,
		ApplicationStatus: &status.StatusInfo{
			Status: status.Unset,
			Since:  ptr(clock.Now()),
		},
		ApplicationConfig: args.ApplicationConfig,
		ApplicationSettings: application.ApplicationSettings{
			Trust: args.Trust,
		},
		Constraints:               args.Constraints,
		StorageDirectiveOverrides: sdo,
	}
	if modelType == coremodel.CAAS {
		unitArgs, err := makeCAASUnitArgs(args)
		if err != nil {
			return errors.Capture(err)
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
			return handleApplicationDomainError(errors.Capture(err))
		}
	}

	unitArgs, err := makeIAASUnitArgs(args)
	if err != nil {
		return errors.Capture(err)
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
		return handleApplicationDomainError(err)
	}

	return nil
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
			return nil, errors.Capture(err)
		}
		pendingResources = append(pendingResources, resUUID)
	}
	return pendingResources, nil
}

// addUnits starts n units of the given application using the specified placement
// directives to allocate the machines.
func (api *APIBase) addUnits(
	ctx context.Context,
	appName string,
	n int,
	placement []*instance.Placement,
	attachStorage []names.StorageTag,
	assignUnits bool,
	charmMeta *charm.Meta,
) ([]coreunit.Name, error) {
	units := make([]coreunit.Name, 0, n)

	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		var unitPlacement *instance.Placement
		if i < len(placement) {
			unitPlacement = placement[i]
		}

		unitArg := applicationservice.AddUnitArg{
			Placement: unitPlacement,
		}

		var unitNames []coreunit.Name
		var err error
		if api.modelType == coremodel.CAAS {
			unitNames, err = api.applicationService.AddCAASUnits(ctx, appName, unitArg)
		} else {
			unitNames, _, err = api.applicationService.AddIAASUnits(ctx, appName, applicationservice.AddIAASUnitArg{
				AddUnitArg: unitArg,
			})
		}
		if err != nil {
			return nil, errors.Errorf("adding unit to application %q: %w", appName, err)
		}
		units = append(units, unitNames...)
	}

	return units, nil
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
		return errors.Errorf("querying feature set supported by the model: %w", err)
	}

	if err = featureSet.Satisfies(assumesExprTree); err != nil {
		return errors.Errorf("not supported %w", err).Add(coreerrors.NotSupported)
	}

	return nil
}
