// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
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
			ApplicationService:      domainServices.Application(),
			CloudService:            domainServices.Cloud(),
			ControllerConfigService: domainServices.ControllerConfig(),
			CredentialService:       domainServices.Credential(),
			MachineService:          domainServices.Machine(),
			ModelConfigService:      domainServices.Config(),
			ModelInfoService:        domainServices.ModelInfo(),
			NetworkService:          domainServices.Network(),
			PortService:             domainServices.Port(),
			RelationService:         domainServices.Relation(),
			SecretService:           domainServices.Secret(),
			UnitStateService:        domainServices.UnitState(),
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
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	st := context.State()
	aClock := context.Clock()
	resources := context.Resources()
	watcherRegistry := context.WatcherRegistry()
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipRevoker, err := context.LeadershipRevoker()
	if err != nil {
		return nil, errors.Trace(err)
	}

	accessUnit := unitcommon.UnitAccessor(authorizer, unitcommon.Backend(st))
	accessApplication := applicationAccessor(authorizer, st)
	accessMachine := machineAccessor(authorizer, st)
	accessCloudSpec := cloudSpecAccessor(authorizer, st)
	accessUnitOrApplication := common.AuthAny(accessUnit, accessApplication)

	// Do not use m for anything other than a EnvironConfigGetterModel.
	// This use will disappear once model is fully gone from state.
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAPI, err := newStorageAPI(
		stateShim{st}, storageAccessor, context.DomainServices().BlockDevice(), resources, accessUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelInfo, err := services.ModelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelTag := names.NewModelTag(modelInfo.UUID.String())

	cloudSpec := cloudspec.NewCloudSpecV2(resources,
		cloudspec.MakeCloudSpecGetterForModel(st,
			services.CloudService,
			services.CredentialService,
			services.ModelConfigService,
		),
		cloudspec.MakeCloudSpecWatcherForModel(st, services.CloudService),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, services.CredentialService),
		common.AuthFuncForTag(modelTag),
	)
	modelConfigWatcher := commonmodel.NewModelConfigWatcher(
		services.ModelConfigService,
		context.WatcherRegistry(),
	)
	logger := context.Logger().Child("uniter")

	extUnitState := common.NewExternalUnitStateAPI(
		services.ControllerConfigService,
		services.UnitStateService,
		st,
		resources,
		authorizer,
		accessUnit,
		logger,
	)

	extLXDProfile := NewExternalLXDProfileAPIv2(
		st,
		services.MachineService,
		context.WatcherRegistry(),
		authorizer,
		accessUnit,
		logger,
		services.ModelInfoService,
		services.ApplicationService,
	)

	statusAPI := NewStatusAPI(
		st,
		services.ApplicationService,
		accessUnitOrApplication,
		leadershipChecker,
		aClock,
	)

	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &UniterAPI{
		APIAddresser:       common.NewAPIAddresser(systemState, resources),
		ModelConfigWatcher: modelConfigWatcher,
		RebootRequester:    common.NewRebootRequester(services.MachineService, accessMachine),
		UnitStateAPI:       extUnitState,
		lxdProfileAPI:      extLXDProfile,
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its application's? This is not a pleasing arrangement.
		StatusAPI: statusAPI,

		environConfigGetterModel: m,
		st:                       st,
		clock:                    aClock,
		auth:                     authorizer,
		resources:                resources,
		leadershipChecker:        leadershipChecker,
		leadershipRevoker:        leadershipRevoker,
		accessUnit:               accessUnit,
		accessApplication:        accessApplication,
		accessUnitOrApplication:  accessUnitOrApplication,
		accessMachine:            accessMachine,
		accessCloudSpec:          accessCloudSpec,
		cloudSpecer:              cloudSpec,
		StorageAPI:               storageAPI,
		logger:                   logger,
		store:                    context.ObjectStore(),
		watcherRegistry:          watcherRegistry,

		applicationService:      services.ApplicationService,
		cloudService:            services.CloudService,
		controllerConfigService: services.ControllerConfigService,
		credentialService:       services.CredentialService,
		machineService:          services.MachineService,
		modelConfigService:      services.ModelConfigService,
		modelInfoService:        services.ModelInfoService,
		networkService:          services.NetworkService,
		portService:             services.PortService,
		relationService:         services.RelationService,
		secretService:           services.SecretService,
		unitStateService:        services.UnitStateService,

		cmrBackend: commoncrossmodel.GetBackend(st),
	}, nil
}
