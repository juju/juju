// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"sync"

	jujucharm "github.com/juju/charm/v12"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
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
	app.EXPECT().Refresh().Return(nil).AnyTimes()
	app.EXPECT().CharmURL().DoAndReturn(func() (string, bool, error) {
		app.mu.Lock()
		defer app.mu.Unlock()
		return app.charmURL, app.charmForced, nil
	}).AnyTimes()
	app.EXPECT().CharmModifiedVersion().DoAndReturn(func() (int, error) {
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

func (ctx *testContext) makeUnit(c *gc.C, unitTag names.UnitTag, l life.Value) *unit {
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
	u.EXPECT().Refresh().Return(nil).AnyTimes()
	u.EXPECT().ProviderID().Return("").AnyTimes()
	u.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesNotStarted, "", nil).AnyTimes()
	u.EXPECT().PrincipalName().Return("u", false, nil).AnyTimes()
	u.EXPECT().EnsureDead().DoAndReturn(func() error {
		u.mu.Lock()
		u.life = life.Dead
		u.mu.Unlock()
		return nil
	}).AnyTimes()
	u.EXPECT().MeterStatus().Return("", "", nil).AnyTimes()
	u.EXPECT().PrivateAddress().Return(dummyPrivateAddress.Value, nil).AnyTimes()
	u.EXPECT().PublicAddress().Return(dummyPublicAddress.Value, nil).AnyTimes()
	u.EXPECT().AvailabilityZone().Return("zone-1", nil).AnyTimes()

	u.EXPECT().SetCharmURL(gomock.Any()).DoAndReturn(func(curl string) error {
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
	u.EXPECT().CharmURL().DoAndReturn(func() (string, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.charmURL, nil
	}).AnyTimes()

	u.EXPECT().DestroyAllSubordinates().DoAndReturn(func() error {
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

	u.EXPECT().Resolved().DoAndReturn(func() params.ResolvedMode {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.resolved
	}).AnyTimes()
	u.EXPECT().ClearResolved().DoAndReturn(func() error {
		u.mu.Lock()
		u.resolved = params.ResolvedNone
		u.mu.Unlock()
		ctx.sendUnitNotify(c, "send clear resolved event")
		return nil
	}).AnyTimes()

	u.EXPECT().HasSubordinates().DoAndReturn(func() (bool, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return u.subordinate != nil, nil
	}).AnyTimes()

	u.EXPECT().SetUnitStatus(gomock.Any(), gomock.Any(), nil).DoAndReturn(func(st status.Status, info string, data map[string]any) error {
		u.mu.Lock()
		u.unitStatus = status.StatusInfo{
			Status:  st,
			Message: info,
			Data:    data,
		}
		u.mu.Unlock()
		return nil
	}).AnyTimes()
	u.EXPECT().SetAgentStatus(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(st status.Status, info string, data map[string]any) error {
		u.mu.Lock()
		u.agentStatus = status.StatusInfo{
			Status:  st,
			Message: info,
			Data:    data,
		}
		u.mu.Unlock()
		return nil
	}).AnyTimes()

	getState := func() (params.UnitStateResult, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		result := params.UnitStateResult{
			UniterState:   ctx.uniterState,
			RelationState: ctx.relationState,
			SecretState:   ctx.secretsState,
		}
		return result, nil
	}
	u.EXPECT().State().DoAndReturn(getState).AnyTimes()

	u.EXPECT().RelationsStatus().DoAndReturn(func() ([]apiuniter.RelationStatus, error) {
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

	u.EXPECT().UnitStatus().DoAndReturn(func() (params.StatusResult, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		return params.StatusResult{
			Status: u.unitStatus.Status.String(),
			Info:   u.unitStatus.Message,
		}, nil
	}).AnyTimes()

	u.EXPECT().Application().DoAndReturn(func() (uniterapi.Application, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		return ctx.app, nil
	}).AnyTimes()

	u.EXPECT().CanApplyLXDProfile().DoAndReturn(func() (bool, error) {
		u.mu.Lock()
		tag, err := u.AssignedMachine()
		c.Assert(err, jc.ErrorIsNil)
		u.mu.Unlock()
		return tag.ContainerType() == "lxd", nil
	}).AnyTimes()
	u.EXPECT().LXDProfileName().DoAndReturn(func() (string, error) {
		ctx.stateMu.Lock()
		defer ctx.stateMu.Unlock()
		return lxdprofile.MatchProfileNameByAppName(ctx.machineProfiles, ctx.app.Tag().Id())
	}).AnyTimes()

	// Add to model.
	u.unitStatus.Status = status.Waiting
	u.unitStatus.Message = status.MessageWaitForMachine
	u.EXPECT().SetUnitStatus(status.Waiting, status.MessageInitializingAgent, nil).DoAndReturn(func(st status.Status, info string, data map[string]any) error {
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
			Name:      "juju-info",
			Role:      "provider",
			Interface: "juju-info",
			Scope:     "container",
		},
	},
}

func subordinateRelationKey(ifce string) string {
	relKey := "logging:logging-directory u:logging-dir"
	if ifce == "juju-info" {
		relKey = "logging:info u:juju-info"
	}
	return relKey
}

type relation struct {
	mu sync.Mutex
	*uniterapi.MockRelation

	life life.Value
}

func (ctx *testContext) makeRelation(c *gc.C, relTag names.RelationTag, l life.Value, otherApp string) *relation {
	r := &relation{
		MockRelation: uniterapi.NewMockRelation(ctx.ctrl),
		life:         l,
	}

	ep, ok := endpointsForTest[relTag.Id()]
	c.Assert(ok, jc.IsTrue)

	relId := int(ctx.relCounter.Add(1))
	r.EXPECT().Tag().Return(relTag).AnyTimes()
	r.EXPECT().String().Return(relTag.Id()).AnyTimes()
	r.EXPECT().Id().Return(relId).AnyTimes()
	r.EXPECT().Life().DoAndReturn(func() life.Value {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.life
	}).AnyTimes()
	r.EXPECT().Refresh().Return(nil).AnyTimes()
	r.EXPECT().Suspended().Return(false).AnyTimes()
	r.EXPECT().UpdateSuspended(false).AnyTimes()
	r.EXPECT().Endpoint().Return(&ep, nil).AnyTimes()
	r.EXPECT().OtherApplication().Return(otherApp).AnyTimes()
	r.EXPECT().SetStatus(gomock.Any()).Return(nil).AnyTimes()

	ctx.api.EXPECT().Relation(relTag).Return(r, nil).AnyTimes()
	ctx.api.EXPECT().RelationById(relId).Return(r, nil).AnyTimes()

	return r
}

type relationUnit struct {
	*uniterapi.MockRelationUnit
}

func (ctx *testContext) makeRelationUnit(c *gc.C, rel *relation, u *unit) *relationUnit {
	ru := &relationUnit{
		MockRelationUnit: uniterapi.NewMockRelationUnit(ctx.ctrl),
	}

	ru.EXPECT().Relation().Return(rel).AnyTimes()
	ep, err := rel.Endpoint()
	c.Assert(err, jc.ErrorIsNil)
	ru.EXPECT().Endpoint().Return(*ep).AnyTimes()

	ru.EXPECT().EnterScope().DoAndReturn(func() error {
		if u.Life() != life.Alive || rel.Life() != life.Alive {
			return &params.Error{Code: params.CodeCannotEnterScope, Message: "cannot enter scope: unit or relation is not alive"}
		}
		u.mu.Lock()
		u.inScope = true
		u.mu.Unlock()
		return nil
	}).AnyTimes()
	ru.EXPECT().LeaveScope().DoAndReturn(func() error {
		u.mu.Lock()
		u.inScope = false
		u.mu.Unlock()
		return nil
	}).AnyTimes()

	return ru
}

type stubLeadershipSettingsAccessor struct {
	results map[string]string
}

func (s *stubLeadershipSettingsAccessor) Read(_ string) (result map[string]string, _ error) {
	return result, nil
}

func (s *stubLeadershipSettingsAccessor) Merge(_, _ string, settings map[string]string) error {
	if s.results == nil {
		s.results = make(map[string]string)
	}
	for k, v := range settings {
		s.results[k] = v
	}
	return nil
}
