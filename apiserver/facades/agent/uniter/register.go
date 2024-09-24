// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	applicationservice "github.com/juju/juju/domain/application/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Uniter", 19, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUniterAPI(stdCtx, ctx)
	}, reflect.TypeOf((*UniterAPI)(nil)))
}

// newUniterAPI creates a new instance of the core Uniter API.
func newUniterAPI(stdCtx context.Context, ctx facade.ModelContext) (*UniterAPI, error) {
	domainServices := ctx.DomainServices()
	modelInfoService := domainServices.ModelInfo()

	backendService := domainServices.SecretBackend()
	secretService := domainServices.Secret(
		secretservice.SecretServiceParams{
			BackendAdminConfigGetter: secretbackendservice.AdminBackendConfigGetterFunc(
				backendService, ctx.ModelUUID(),
			),
			BackendUserSecretConfigGetter: secretbackendservice.UserSecretBackendConfigGetterFunc(
				backendService, ctx.ModelUUID(),
			),
		},
	)
	applicationService := domainServices.Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		Secrets:         secretService,
	})

	return newUniterAPIWithServices(
		stdCtx, ctx,
		domainServices.ControllerConfig(),
		domainServices.Config(),
		modelInfoService,
		secretService,
		domainServices.Network(),
		domainServices.Machine(),
		domainServices.Cloud(),
		domainServices.Credential(),
		applicationService,
		domainServices.UnitState(),
	)
}

// newUniterAPIWithServices creates a new instance using the services.
func newUniterAPIWithServices(
	stdCtx context.Context,
	context facade.ModelContext,
	controllerConfigService ControllerConfigService,
	modelConfigService ModelConfigService,
	modelInfoService ModelInfoService,
	secretService SecretService,
	networkService NetworkService,
	machineService MachineService,
	cloudService CloudService,
	credentialService CredentialService,
	applicationService ApplicationService,
	unitStateService UnitStateService,
) (*UniterAPI, error) {
	authorizer := context.Auth()
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	st := context.State()
	aClock := context.StatePool().Clock()
	resources := context.Resources()
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

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st, modelConfigService)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAPI, err := newStorageAPI(
		stateShim{st}, storageAccessor, context.DomainServices().BlockDevice(), resources, accessUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelTag := names.NewModelTag(modelInfo.UUID.String())

	cloudSpec := cloudspec.NewCloudSpecV2(resources,
		cloudspec.MakeCloudSpecGetterForModel(st, cloudService, credentialService, modelConfigService),
		cloudspec.MakeCloudSpecWatcherForModel(st, cloudService),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, credentialService),
		common.AuthFuncForTag(modelTag),
	)

	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger := context.Logger().Child("uniter")
	return &UniterAPI{
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrApplication),
		APIAddresser:               common.NewAPIAddresser(systemState, resources),
		ModelConfigWatcher:         common.NewModelConfigWatcher(modelConfigService, context.WatcherRegistry()),
		RebootRequester:            common.NewRebootRequester(machineService, accessMachine),
		UnitStateAPI:               common.NewExternalUnitStateAPI(controllerConfigService, unitStateService, st, resources, authorizer, accessUnit, logger),
		LeadershipSettingsAccessor: leadershipSettingsAccessorFactory(st, leadershipChecker, resources, authorizer),
		lxdProfileAPI:              NewExternalLXDProfileAPIv2(st, resources, authorizer, accessUnit, logger, modelInfoService),
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its application's? This is not a pleasing arrangement.
		StatusAPI: NewStatusAPI(m, accessUnitOrApplication, leadershipChecker),

		m:                       m,
		st:                      st,
		controllerConfigService: controllerConfigService,
		modelConfigService:      modelConfigService,
		modelInfoService:        modelInfoService,
		secretService:           secretService,
		networkService:          networkService,
		cloudService:            cloudService,
		credentialService:       credentialService,
		applicationService:      applicationService,
		unitStateService:        unitStateService,
		clock:                   aClock,
		auth:                    authorizer,
		resources:               resources,
		leadershipChecker:       leadershipChecker,
		leadershipRevoker:       leadershipRevoker,
		accessUnit:              accessUnit,
		accessApplication:       accessApplication,
		accessUnitOrApplication: accessUnitOrApplication,
		accessMachine:           accessMachine,
		accessCloudSpec:         accessCloudSpec,
		cloudSpecer:             cloudSpec,
		StorageAPI:              storageAPI,
		logger:                  logger,
		store:                   context.ObjectStore(),
	}, nil
}
