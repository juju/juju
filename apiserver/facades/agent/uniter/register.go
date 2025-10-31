// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Uniter", 19, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUniterAPIv19(stdCtx, ctx)
	}, reflect.TypeOf((*UniterAPIv19)(nil)))
	registry.MustRegister("Uniter", 20, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUniterAPIv20(stdCtx, ctx)
	}, reflect.TypeOf((*UniterAPIv20)(nil)))
	registry.MustRegister("Uniter", 21, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUniterAPI(stdCtx, ctx)
	}, reflect.TypeOf((*UniterAPI)(nil)))
}

func newUniterAPIv19(stdCtx context.Context, ctx facade.ModelContext) (*UniterAPIv19, error) {
	api, err := newUniterAPIv20(stdCtx, ctx)
	if err != nil {
		return nil, err
	}
	return &UniterAPIv19{UniterAPIv20: api}, nil
}

func newUniterAPIv20(stdCtx context.Context, ctx facade.ModelContext) (*UniterAPIv20, error) {
	api, err := newUniterAPI(stdCtx, ctx)
	if err != nil {
		return nil, err
	}
	return &UniterAPIv20{UniterAPI: api}, nil
}

// newUniterAPI creates a new instance of the core Uniter API.
func newUniterAPI(stdCtx context.Context, ctx facade.ModelContext) (*UniterAPI, error) {
	domainServices := ctx.DomainServices()

	return newUniterAPIWithServices(
		stdCtx, ctx,
		Services{
			ApplicationService:         domainServices.Application(),
			BlockDeviceService:         domainServices.BlockDevice(),
			ResolveService:             domainServices.Resolve(),
			StatusService:              domainServices.Status(),
			ControllerConfigService:    domainServices.ControllerConfig(),
			ControllerNodeService:      domainServices.ControllerNode(),
			MachineService:             domainServices.Machine(),
			ModelConfigService:         domainServices.Config(),
			ModelInfoService:           domainServices.ModelInfo(),
			ModelProviderService:       domainServices.ModelProvider(),
			NetworkService:             domainServices.Network(),
			OperationService:           domainServices.Operation(),
			PortService:                domainServices.Port(),
			RelationService:            domainServices.Relation(),
			RemovalService:             domainServices.Removal(),
			SecretService:              domainServices.Secret(),
			StorageProvisioningService: domainServices.StorageProvisioning(),
			UnitStateService:           domainServices.UnitState(),
		},
	)
}

// newUniterAPIWithServices creates a new instance using the services.
func newUniterAPIWithServices(
	stdCtx context.Context,
	context facade.ModelContext,
	services Services,
) (*UniterAPI, error) {
	authorizer := context.Auth()
	if !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	aClock := context.Clock()
	watcherRegistry := context.WatcherRegistry()
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipRevoker, err := context.LeadershipRevoker()
	if err != nil {
		return nil, errors.Trace(err)
	}

	accessUnit := unitcommon.UnitAccessor(authorizer, services.ApplicationService)
	accessApplication := applicationAccessor(authorizer)
	accessMachine := machineAccessor(authorizer, services.ApplicationService)
	accessCloudSpec := cloudSpecAccessor(authorizer, services.ApplicationService)
	accessUnitOrApplication := common.AuthAny(accessUnit, accessApplication)

	modelInfo, err := services.ModelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAPI, err := newStorageAPI(
		services.BlockDeviceService,
		services.ApplicationService,
		services.RemovalService,
		services.StorageProvisioningService,
		watcherRegistry,
		accessUnit,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelConfigWatcher := commonmodel.NewModelConfigWatcher(
		services.ModelConfigService,
		watcherRegistry,
	)
	logger := context.Logger().Child("uniter")

	unitState := common.NewUnitStateAPI(
		services.ControllerConfigService,
		services.UnitStateService,
		accessUnit,
		logger,
	)

	extLXDProfile := NewExternalLXDProfileAPI(
		services.MachineService,
		watcherRegistry,
		authorizer,
		accessUnit,
		logger,
		services.ModelInfoService,
		services.ApplicationService,
	)

	statusAPI := NewStatusAPI(
		services.StatusService,
		accessUnitOrApplication,
		leadershipChecker,
		aClock,
	)

	return &UniterAPI{
		APIAddresser:       common.NewAPIAddresser(services.ControllerNodeService, watcherRegistry),
		ModelConfigWatcher: modelConfigWatcher,
		RebootRequester:    common.NewRebootRequester(services.MachineService, accessMachine),
		UnitStateAPI:       unitState,
		lxdProfileAPI:      extLXDProfile,
		StatusAPI:          statusAPI,

		modelUUID:               context.ModelUUID(),
		modelType:               modelInfo.Type,
		clock:                   aClock,
		auth:                    authorizer,
		leadershipChecker:       leadershipChecker,
		leadershipRevoker:       leadershipRevoker,
		accessUnit:              accessUnit,
		accessApplication:       accessApplication,
		accessUnitOrApplication: accessUnitOrApplication,
		accessMachine:           accessMachine,
		accessCloudSpec:         accessCloudSpec,
		StorageAPI:              storageAPI,
		logger:                  logger,
		store:                   context.ObjectStore(),
		watcherRegistry:         watcherRegistry,

		applicationService:      services.ApplicationService,
		controllerConfigService: services.ControllerConfigService,
		machineService:          services.MachineService,
		modelConfigService:      services.ModelConfigService,
		modelInfoService:        services.ModelInfoService,
		modelProviderService:    services.ModelProviderService,
		networkService:          services.NetworkService,
		operationService:        services.OperationService,
		portService:             services.PortService,
		relationService:         services.RelationService,
		removalService:          services.RemovalService,
		resolveService:          services.ResolveService,
		statusService:           services.StatusService,
		secretService:           services.SecretService,
		unitStateService:        services.UnitStateService,
	}, nil
}
