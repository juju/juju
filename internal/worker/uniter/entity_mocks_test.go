// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	jujucharm "github.com/juju/juju/internal/charm"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/rpc/params"
)

var (
	dummyPrivateAddress = network.NewSpaceAddress("172.0.30.1", network.WithScope(network.ScopeCloudLocal))
	dummyPublicAddress  = network.NewSpaceAddress("1.1.1.1", network.WithScope(network.ScopePublic))
)

type application struct {
	mu sync.Mutex
	*uniterapi.MockApplication

	charmURL             string
	charmForced          bool
	charmModifiedVersion int
	config               map[string]any
}

func (app *application) String() string {
	return app.MockApplication.Tag().Id()
}

func (ctx *testContext) makeApplication(appTag names.ApplicationTag) *application {
	app := &application{
		MockApplication: uniterapi.NewMockApplication(ctx.ctrl),
		charmURL:        curl(0),
	}

	app.EXPECT().Tag().Return(appTag).AnyTimes()
	app.EXPECT().Life().Return(life.Alive).AnyTimes()
	app.EXPECT().Refresh(gomock.Any()).Return(nil).AnyTimes()
	app.EXPECT().CharmURL(gomock.Any()).DoAndReturn(func(context.Context) (string, bool, error) {
		app.mu.Lock()
		defer app.mu.Unlock()
		return app.charmURL, app.charmForced, nil
	}).AnyTimes()
	app.EXPECT().CharmModifiedVersion(gomock.Any()).DoAndReturn(func(context.Context) (int, error) {
		app.mu.Lock()
		defer app.mu.Unlock()
		return app.charmModifiedVersion, nil
	}).AnyTimes()

	return app
}

func (app *application) configHash(newCfg map[string]any) string {
	app.mu.Lock()
	defer app.mu.Unlock()
	if newCfg != nil {
		app.config = newCfg
	}
	return fmt.Sprintf("%s:%d:%s", app.charmURL, app.charmModifiedVersion, app.config)
}

type unit struct {
	mu sync.Mutex
	*uniterapi.MockUnit

	subordinate *unit

	charmURL    string
	life        life.Value
	unitStatus  status.StatusInfo
	agentStatus status.StatusInfo
	inScope     bool
	resolved    params.ResolvedMode
}

func (u *unit) String() string {
	return u.MockUnit.Name()
}

func (ctx *testContext) makeUnit(c tc.LikeC, unitTag names.UnitTag, l life.Value) *unit {
	u := &unit{
		MockUnit: uniterapi.NewMockUnit(ctx.ctrl),
		life:     l,
		charmURL: curl(0),
	}

	appName, _ := names.UnitApplication(unitTag.Id())
	appTag := names.NewApplicationTag(appName)
	u.EXPECT().Name().Return(unitTag.Id()).AnyTimes()
	u.EXPECT().Tag().Return(unitTag).AnyTimes()
	u.EXPECT().ApplicationTag().Return(appTag).AnyTimes()
	u.EXPECT().Refresh(gomock.Any()).Return(nil).AnyTimes()
	u.EXPECT().ProviderID().Return("").AnyTimes()
	u.EXPECT().PrincipalName(gomock.Any()).Return("u", false, nil).AnyTimes()
	u.EXPECT().EnsureDead(gomock.Any()).DoAndReturn(func(context.Context) error {
		u.mu.Lock()
		u.life = life.Dead
		u.mu.Unlock()
		return nil
	}).AnyTimes()
	u.EXPECT().PrivateAddress(gomock.Any()).Return(dummyPrivateAddress.Value, nil).AnyTimes()
	u.EXPECT().PublicAddress(gomock.Any()).Return(dummyPublicAddress.Value, nil).AnyTimes()
	u.EXPECT().AvailabilityZone(gomock.Any()).Return("zone-1", nil).AnyTimes()

	u.EXPECT().SetCharmURL(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, curl string) error {
		u.mu.Lock()
		defer u.mu.Unlock()
		u.charmURL = curl
		return nil
	}).AnyTimes()

	u.EXPECT().Life().DoAndReturn(func() life.Value {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.life
	}).AnyTimes()
	u.EXPECT().CharmURL(gomock.Any()).DoAndReturn(func(context.Context) (string, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.charmURL, nil
	}).AnyTimes()

	u.EXPECT().DestroyAllSubordinates(gomock.Any()).DoAndReturn(func(context.Context) error {
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.subordinate == nil {
			return nil
		}
		u.subordinate.mu.Lock()
		u.subordinate.life = life.Dying
		u.subordinate.mu.Unlock()
		return nil
	}).AnyTimes()

	u.EXPECT().Resolved(gomock.Any()).DoAndReturn(func(context.Context) (params.ResolvedMode, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.resolved, nil
	}).AnyTimes()
	u.EXPECT().ClearResolved(gomock.Any()).DoAndReturn(func(context.Context) error {
		u.mu.Lock()
		u.resolved = params.ResolvedNone
		u.mu.Unlock()
		ctx.sendNotify(c, ctx.unitResolveCh, "send clear resolved event")
		return nil
	}).AnyTimes()

	u.EXPECT().HasSubordinates(gomock.Any()).DoAndReturn(func(context.Context) (bool, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.subordinate != nil, nil
	}).AnyTimes()

	u.EXPECT().SetUnitStatus(gomock.Any(), gomock.Any(), gomock.Any(), nil).DoAndReturn(func(_ context.Context, st status.Status, info string, data map[string]any) error {
		u.mu.Lock()
		u.unitStatus = status.StatusInfo{
			Status:  st,
			Message: info,
			Data:    data,
		}
		u.mu.Unlock()
		return nil
	}).AnyTimes()
	u.EXPECT().SetAgentStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, st status.Status, info string, data map[string]any) error {
		u.mu.Lock()
		u.agentStatus = status.StatusInfo{
			Status:  st,
			Message: info,
			Data:    data,
		}
		u.mu.Unlock()
		return nil
	}).AnyTimes()

	getState := func(context.Context) (params.UnitStateResult, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		result := params.UnitStateResult{
			UniterState:   ctx.uniterState,
			RelationState: ctx.relationState,
			SecretState:   ctx.secretsState,
		}
		return result, nil
	}
	u.EXPECT().State(gomock.Any()).DoAndReturn(getState).AnyTimes()

	u.EXPECT().RelationsStatus(gomock.Any()).DoAndReturn(func(context.Context) ([]apiuniter.RelationStatus, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		var result []apiuniter.RelationStatus
		if ctx.relation != nil {
			result = []apiuniter.RelationStatus{{
				Tag:       ctx.relation.Tag(),
				Suspended: false,
				InScope:   u.inScope,
			}}
		}
		return result, nil
	}).AnyTimes()

	u.EXPECT().UnitStatus(gomock.Any()).DoAndReturn(func(context.Context) (params.StatusResult, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return params.StatusResult{
			Status: u.unitStatus.Status.String(),
			Info:   u.unitStatus.Message,
		}, nil
	}).AnyTimes()

	u.EXPECT().Application(gomock.Any()).DoAndReturn(func(context.Context) (uniterapi.Application, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		return ctx.app, nil
	}).AnyTimes()

	u.EXPECT().CanApplyLXDProfile(gomock.Any()).DoAndReturn(func(context.Context) (bool, error) {
		u.mu.Lock()
		tag, err := u.AssignedMachine(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		u.mu.Unlock()
		return tag.ContainerType() == "lxd", nil
	}).AnyTimes()
	u.EXPECT().LXDProfileName(gomock.Any()).DoAndReturn(func(context.Context) (string, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		return lxdprofile.MatchProfileNameByAppName(ctx.machineProfiles, ctx.app.Tag().Id())
	}).AnyTimes()

	// Add to model.
	u.unitStatus.Status = status.Waiting
	u.unitStatus.Message = status.MessageWaitForMachine
	u.EXPECT().SetUnitStatus(gomock.Any(), status.Waiting, status.MessageInitializingAgent, nil).DoAndReturn(func(_ context.Context, st status.Status, info string, data map[string]any) error {
		u.mu.Lock()
		u.unitStatus = status.StatusInfo{
			Status:  st,
			Message: info,
			Data:    data,
		}
		u.mu.Unlock()
		return nil
	}).MaxTimes(1)

	return u
}

// endpointsForTest replaces code from state which we don't run here,
// and instead provides some hard coded tests data to match the charms
// used in the tests.
var endpointsForTest = map[string]apiuniter.Endpoint{
	"wordpress:db mysql:db": {
		Relation: jujucharm.Relation{
			Name:      "db",
			Role:      "requirer",
			Interface: "mysql",
			Scope:     "global",
		},
	},
	"logging:logging-directory u:logging-dir": {
		Relation: jujucharm.Relation{
			Name:      "logging-dir",
			Role:      "provider",
			Interface: "logging-dir",
			Scope:     "container",
		},
	},
	"logging:info u:juju-info": {
		Relation: jujucharm.Relation{
			Name:      corerelation.JujuInfo,
			Role:      "provider",
			Interface: corerelation.JujuInfo,
			Scope:     "container",
		},
	},
}

func subordinateRelationKey(ifce string) string {
	relKey := "logging:logging-directory u:logging-dir"
	if ifce == corerelation.JujuInfo {
		relKey = "logging:info u:juju-info"
	}
	return relKey
}

type relation struct {
	mu sync.Mutex
	*uniterapi.MockRelation

	life life.Value
}

func (ctx *testContext) makeRelation(c tc.LikeC, relTag names.RelationTag, l life.Value, otherApp string) *relation {
	r := &relation{
		MockRelation: uniterapi.NewMockRelation(ctx.ctrl),
		life:         l,
	}

	ep, ok := endpointsForTest[relTag.Id()]
	c.Assert(ok, tc.IsTrue)

	relId := int(ctx.relCounter.Add(1))
	r.EXPECT().Tag().Return(relTag).AnyTimes()
	r.EXPECT().String().Return(relTag.Id()).AnyTimes()
	r.EXPECT().Id().Return(relId).AnyTimes()
	r.EXPECT().Life().DoAndReturn(func() life.Value {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.life
	}).AnyTimes()
	r.EXPECT().Refresh(gomock.Any()).Return(nil).AnyTimes()
	r.EXPECT().Suspended().Return(false).AnyTimes()
	r.EXPECT().UpdateSuspended(false).AnyTimes()
	r.EXPECT().Endpoint(gomock.Any()).Return(&ep, nil).AnyTimes()
	r.EXPECT().OtherApplication().Return(otherApp).AnyTimes()
	r.EXPECT().SetStatus(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	ctx.api.EXPECT().Relation(gomock.Any(), relTag).Return(r, nil).AnyTimes()
	ctx.api.EXPECT().RelationById(gomock.Any(), relId).Return(r, nil).AnyTimes()

	return r
}

type relationUnit struct {
	*uniterapi.MockRelationUnit
}

func (ctx *testContext) makeRelationUnit(c tc.LikeC, rel *relation, u *unit) *relationUnit {
	ru := &relationUnit{
		MockRelationUnit: uniterapi.NewMockRelationUnit(ctx.ctrl),
	}

	ru.EXPECT().Relation().Return(rel).AnyTimes()
	ep, err := rel.Endpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	ru.EXPECT().Endpoint().Return(*ep).AnyTimes()

	ru.EXPECT().EnterScope(gomock.Any()).DoAndReturn(func(context.Context) error {
		if u.Life() != life.Alive || rel.Life() != life.Alive {
			return &params.Error{Code: params.CodeCannotEnterScope, Message: "cannot enter scope: unit or relation is not alive"}
		}
		u.mu.Lock()
		u.inScope = true
		u.mu.Unlock()
		return nil
	}).AnyTimes()
	ru.EXPECT().LeaveScope(gomock.Any()).DoAndReturn(func(context.Context) error {
		u.mu.Lock()
		u.inScope = false
		u.mu.Unlock()
		return nil
	}).AnyTimes()

	return ru
}
