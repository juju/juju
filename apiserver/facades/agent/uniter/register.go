// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/common/cloudspec"
	"github.com/juju/juju/v2/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/apiserver/facades/agent/meterstatus"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Uniter", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV4(ctx)
	}, reflect.TypeOf((*UniterAPIV4)(nil)))
	registry.MustRegister("Uniter", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV5(ctx)
	}, reflect.TypeOf((*UniterAPIV5)(nil)))
	registry.MustRegister("Uniter", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV6(ctx)
	}, reflect.TypeOf((*UniterAPIV6)(nil)))
	registry.MustRegister("Uniter", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV7(ctx)
	}, reflect.TypeOf((*UniterAPIV7)(nil)))
	registry.MustRegister("Uniter", 8, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV8(ctx)
	}, reflect.TypeOf((*UniterAPIV8)(nil)))
	registry.MustRegister("Uniter", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV9(ctx)
	}, reflect.TypeOf((*UniterAPIV9)(nil)))
	registry.MustRegister("Uniter", 10, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV10(ctx)
	}, reflect.TypeOf((*UniterAPIV10)(nil)))
	registry.MustRegister("Uniter", 11, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV11(ctx)
	}, reflect.TypeOf((*UniterAPIV11)(nil)))
	registry.MustRegister("Uniter", 12, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV12(ctx)
	}, reflect.TypeOf((*UniterAPIV12)(nil)))
	registry.MustRegister("Uniter", 13, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV13(ctx)
	}, reflect.TypeOf((*UniterAPIV13)(nil)))
	registry.MustRegister("Uniter", 14, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV14(ctx)
	}, reflect.TypeOf((*UniterAPIV14)(nil)))
	registry.MustRegister("Uniter", 15, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV15(ctx)
	}, reflect.TypeOf((*UniterAPIV15)(nil)))
	registry.MustRegister("Uniter", 16, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV16(ctx)
	}, reflect.TypeOf((*UniterAPIV16)(nil)))
	registry.MustRegister("Uniter", 17, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPIV17(ctx)
	}, reflect.TypeOf((*UniterAPIV17)(nil)))
	registry.MustRegister("Uniter", 18, func(ctx facade.Context) (facade.Facade, error) {
		return newUniterAPI(ctx)
	}, reflect.TypeOf((*UniterAPI)(nil)))
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
	return &UniterAPI{
		LifeGetter:                 common.NewLifeGetter(st, accessUnitOrApplication),
		DeadEnsurer:                common.NewDeadEnsurer(st, common.RevokeLeadershipFunc(leadershipRevoker), accessUnit),
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrApplication),
		APIAddresser:               common.NewAPIAddresser(systemState, resources),
		ModelWatcher:               common.NewModelWatcher(m, resources, authorizer),
		RebootRequester:            common.NewRebootRequester(st, accessMachine),
		UpgradeSeriesAPI:           common.NewExternalUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
		UnitStateAPI:               common.NewExternalUnitStateAPI(st, resources, authorizer, accessUnit, logger),
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

// newUniterAPIV17 creates an instance of the V17 uniter API.
func newUniterAPIV17(context facade.Context) (*UniterAPIV17, error) {
	uniterAPI, err := newUniterAPI(context)
	if err != nil {
		return nil, err
	}

	m, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	uniterAPI.cloudSpecer = cloudspec.NewCloudSpecV1(context.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(context.State()),
		cloudspec.MakeCloudSpecWatcherForModel(context.State()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(context.State()),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(context.State()),
		common.AuthFuncForTag(m.ModelTag()),
	)
	return &UniterAPIV17{
		UniterAPI: *uniterAPI,
	}, nil
}

// newUniterAPIV16 creates an instance of the V16 uniter API.
func newUniterAPIV16(context facade.Context) (*UniterAPIV16, error) {
	uniterAPI, err := newUniterAPIV17(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV16{
		UniterAPIV17: *uniterAPI,
	}, nil
}

// newUniterAPIV15 creates an instance of the V15 uniter API.
func newUniterAPIV15(context facade.Context) (*UniterAPIV15, error) {
	uniterAPI, err := newUniterAPIV16(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV15{
		UniterAPIV16: *uniterAPI,
	}, nil
}

// newUniterAPIV14 creates an instance of the V14 uniter API.
func newUniterAPIV14(context facade.Context) (*UniterAPIV14, error) {
	uniterAPI, err := newUniterAPIV15(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV14{
		UniterAPIV15: *uniterAPI,
	}, nil
}

// newUniterAPIV13 creates an instance of the V13 uniter API.
func newUniterAPIV13(context facade.Context) (*UniterAPIV13, error) {
	uniterAPI, err := newUniterAPIV14(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV13{
		UniterAPIV14: *uniterAPI,
	}, nil
}

// newUniterAPIV12 creates an instance of the V12 uniter API.
func newUniterAPIV12(context facade.Context) (*UniterAPIV12, error) {
	uniterAPI, err := newUniterAPIV13(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV12{
		UniterAPIV13: *uniterAPI,
	}, nil
}

// newUniterAPIV11 creates an instance of the V11 uniter API.
func newUniterAPIV11(context facade.Context) (*UniterAPIV11, error) {
	uniterAPI, err := newUniterAPIV12(context)
	if err != nil {
		return nil, err
	}
	authorizer := context.Auth()
	st := context.State()
	resources := context.Resources()
	accessUnit := unitcommon.UnitAccessor(authorizer, unitcommon.Backend(st))
	return &UniterAPIV11{
		LXDProfileAPI: NewExternalLXDProfileAPI(st, resources, authorizer, accessUnit, logger),
		UniterAPIV12:  *uniterAPI,
	}, nil
}

// newUniterAPIV10 creates an instance of the V10 uniter API.
func newUniterAPIV10(context facade.Context) (*UniterAPIV10, error) {
	uniterAPI, err := newUniterAPIV11(context)
	if err != nil {
		return nil, err
	}

	return &UniterAPIV10{
		LXDProfileAPI: uniterAPI.LXDProfileAPI,
		UniterAPIV11:  *uniterAPI,
	}, nil
}

// newUniterAPIV9 creates an instance of the V9 uniter API.
func newUniterAPIV9(context facade.Context) (*UniterAPIV9, error) {
	uniterAPI, err := newUniterAPIV10(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV9{
		LXDProfileAPI: uniterAPI.LXDProfileAPI,
		UniterAPIV10:  *uniterAPI,
	}, nil
}

// newUniterAPIV8 creates an instance of the V8 uniter API.
func newUniterAPIV8(context facade.Context) (*UniterAPIV8, error) {
	uniterAPI, err := newUniterAPIV9(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV8{
		UniterAPIV9: *uniterAPI,
	}, nil
}

// newUniterAPIV7 creates an instance of the V7 uniter API.
func newUniterAPIV7(context facade.Context) (*UniterAPIV7, error) {
	uniterAPI, err := newUniterAPIV8(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV7{
		UniterAPIV8: *uniterAPI,
	}, nil
}

// newUniterAPIV6 creates an instance of the V6 uniter API.
func newUniterAPIV6(context facade.Context) (*UniterAPIV6, error) {
	uniterAPI, err := newUniterAPIV7(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV6{
		UniterAPIV7: *uniterAPI,
	}, nil
}

// newUniterAPIV5 creates an instance of the V5 uniter API.
func newUniterAPIV5(context facade.Context) (*UniterAPIV5, error) {
	uniterAPI, err := newUniterAPIV6(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV5{
		UniterAPIV6: *uniterAPI,
	}, nil
}

// newUniterAPIV4 creates an instance of the V4 uniter API.
func newUniterAPIV4(context facade.Context) (*UniterAPIV4, error) {
	uniterAPI, err := newUniterAPIV5(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV4{
		UniterAPIV5: *uniterAPI,
	}, nil
}
