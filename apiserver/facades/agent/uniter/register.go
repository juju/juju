// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/leadership"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Uniter", 18, func(ctx facade.Context) (facade.Facade, error) {
		return NewUniterFacade(ctx)
	}, reflect.TypeOf((*UniterAPI)(nil)))
}

// NewUniterFacade provides the signature required for facade registration.
func NewUniterFacade(context facade.Context) (*UniterAPI, error) {
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
	leadershipRevoker, err := context.LeadershipRevoker(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	accessUnit := unitcommon.UnitAccessor(authorizer, unitcommon.Backend(st))
	accessApplication := ApplicationAccessor(authorizer, st)
	accessMachine := MachineAccessor(authorizer, st)
	accessCloudSpec := CloudSpecAccessor(authorizer, st)

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := GetStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAPI, err := newStorageAPI(
		StateShim{st}, storageAccessor, resources, accessUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService := context.ServiceFactory().ControllerConfig()
	msAPI, err := meterstatus.NewMeterStatusAPI(
		controllerConfigService, st,
		resources, authorizer,
		context.Logger().Child("meterstatus"),
	)
	if err != nil {
		return nil, errors.Annotate(err, "could not create meter status API handler")
	}
	accessUnitOrApplication := common.AuthAny(accessUnit, accessApplication)

	cloudSpec := cloudspec.NewCloudSpecV2(resources,
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(m.ModelTag()),
	)

	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretsAPI, err := secretsmanager.NewSecretManagerAPI(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger := context.Logger().Child("uniter")
	return newUniterAPI(
		common.NewLifeGetter(st, accessUnitOrApplication),
		common.NewDeadEnsurer(st, common.RevokeLeadershipFunc(leadershipRevoker), accessUnit),
		common.NewAgentEntityWatcher(st, resources, accessUnitOrApplication),
		common.NewAPIAddresser(systemState, resources),
		common.NewModelWatcher(m, resources, authorizer),
		common.NewRebootRequester(st, accessMachine),
		common.NewExternalUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
		common.NewExternalUnitStateAPI(st, resources, authorizer, accessUnit, logger),
		secretsAPI,
		LeadershipSettingsAccessorFactory(st, leadershipChecker, resources, authorizer),
		msAPI,
		NewExternalLXDProfileAPIv2(st, resources, authorizer, accessUnit, logger),
		NewStatusAPI(m, accessUnitOrApplication, leadershipChecker),
		m, st, aClock, context.Cancel(),
		authorizer, resources, leadershipChecker,
		accessUnit, accessApplication, accessMachine, accessCloudSpec,
		cloudSpec, storageAPI, logger,
		controllerConfigService,
	)
}

// newUniterAPI creates a new instance of the core Uniter API.
func newUniterAPI(
	lifeGetter *common.LifeGetter,
	deadEnsurer *common.DeadEnsurer,
	agentEntityWatcher *common.AgentEntityWatcher,
	apiAddresser *common.APIAddresser,
	modelWatcher *common.ModelWatcher,
	rebootRequester *common.RebootRequester,
	upgradeSeriesAPI *common.UpgradeSeriesAPI,
	unitStateAPI *common.UnitStateAPI,
	secretsAPI *secretsmanager.SecretsManagerAPI,
	leadershipSettingsAccessor *leadership.LeadershipSettingsAccessor,
	meterStatusAPI *meterstatus.MeterStatusAPI,
	lxdProfileAPI *LXDProfileAPIv2,
	statusAPI *StatusAPI,
	m *state.Model,
	st *state.State,
	clock clock.Clock,
	cancel <-chan struct{},
	authorizer facade.Authorizer,
	resources facade.Resources,
	leadershipChecker coreleadership.Checker,
	accessUnit, accessApplication, accessMachine common.GetAuthFunc,
	accessCloudSpec func() (func() bool, error),
	cloudSpec cloudspec.CloudSpecer,
	storageAPI *StorageAPI,
	logger loggo.Logger,
	controllerConfigGetter ControllerConfigGetter,
) (*UniterAPI, error) {
	return &UniterAPI{
		LifeGetter:                 lifeGetter,
		DeadEnsurer:                deadEnsurer,
		AgentEntityWatcher:         agentEntityWatcher,
		APIAddresser:               apiAddresser,
		ModelWatcher:               modelWatcher,
		RebootRequester:            rebootRequester,
		UpgradeSeriesAPI:           upgradeSeriesAPI,
		UnitStateAPI:               unitStateAPI,
		SecretsManagerAPI:          secretsAPI,
		LeadershipSettingsAccessor: leadershipSettingsAccessor,
		MeterStatus:                meterStatusAPI,
		lxdProfileAPI:              lxdProfileAPI,
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its application's? This is not a pleasing arrangement.
		StatusAPI: statusAPI,

		m:                 m,
		st:                st,
		clock:             clock,
		cancel:            cancel,
		auth:              authorizer,
		resources:         resources,
		leadershipChecker: leadershipChecker,
		accessUnit:        accessUnit,
		accessApplication: accessApplication,
		accessMachine:     accessMachine,
		accessCloudSpec:   accessCloudSpec,
		cloudSpecer:       cloudSpec,
		StorageAPI:        storageAPI,
		logger:            logger,

		controllerConfigGetter: controllerConfigGetter,
	}, nil
}
