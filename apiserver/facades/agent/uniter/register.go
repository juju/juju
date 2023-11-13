// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "Uniter",
		Versions: facades.FacadeVersion{18, 19},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Uniter", 18, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIv18(ctx)
	}, reflect.TypeOf((*UniterAPIv18)(nil)))
	registry.MustRegister("Uniter", 19, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPI(ctx)
	}, reflect.TypeOf((*UniterAPI)(nil)))
}

func newUniterAPIv18(context facade.Context) (*UniterAPIv18, error) {
	api, err := newUniterAPI(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &UniterAPIv18{*api}, nil
}

// newUniterAPI creates a new instance of the core Uniter API.
func newUniterAPI(context facade.Context) (*UniterAPI, error) {
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
	accessApplication := applicationAccessor(authorizer, st)
	accessMachine := machineAccessor(authorizer, st)
	accessCloudSpec := cloudSpecAccessor(authorizer, st)

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAPI, err := newStorageAPI(
		stateShim{st}, storageAccessor, resources, accessUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	msAPI, err := meterstatus.NewMeterStatusAPI(st, resources, authorizer)
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

	cacheModel, err := context.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, err
	}

	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretsAPI, err := secretsmanager.NewSecretManagerAPI(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &UniterAPI{
		LifeGetter:                 common.NewLifeGetter(st, accessUnitOrApplication),
		DeadEnsurer:                common.NewDeadEnsurer(st, common.RevokeLeadershipFunc(leadershipRevoker), accessUnit),
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrApplication),
		APIAddresser:               common.NewAPIAddresser(systemState, resources),
		ModelWatcher:               common.NewModelWatcher(m, resources, authorizer),
		RebootRequester:            common.NewRebootRequester(st, accessMachine),
		UpgradeSeriesAPI:           common.NewExternalUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
		UnitStateAPI:               common.NewExternalUnitStateAPI(st, resources, authorizer, accessUnit, logger),
		SecretsManagerAPI:          secretsAPI,
		LeadershipSettingsAccessor: leadershipSettingsAccessorFactory(st, leadershipChecker, resources, authorizer),
		MeterStatus:                msAPI,
		lxdProfileAPI:              NewExternalLXDProfileAPIv2(st, resources, authorizer, accessUnit, logger),
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its application's? This is not a pleasing arrangement.
		StatusAPI: NewStatusAPI(st, &cacheShim{cacheModel}, accessUnitOrApplication, leadershipChecker),

		m:                 m,
		st:                st,
		clock:             aClock,
		cancel:            context.Cancel(),
		cacheModel:        cacheModel,
		auth:              authorizer,
		resources:         resources,
		leadershipChecker: leadershipChecker,
		accessUnit:        accessUnit,
		accessApplication: accessApplication,
		accessMachine:     accessMachine,
		accessCloudSpec:   accessCloudSpec,
		cloudSpecer:       cloudSpec,
		StorageAPI:        storageAPI,
	}, nil
}
