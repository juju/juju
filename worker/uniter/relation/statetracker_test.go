// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/relation/mocks"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type stateTrackerSuite struct {
	baseStateTrackerSuite
}

type baseStateTrackerSuite struct {
	leadershipContext context.LeadershipContext
	unitTag           names.UnitTag
	unitChanges       chan struct{}

	state        *mocks.MockStateTrackerState
	unit         *mocks.MockUnit
	relation     *mocks.MockRelation
	relationer   *mocks.MockRelationer
	relationUnit *mocks.MockRelationUnit
	stateMgr     *mocks.MockStateManager
	watcher      *watchertest.MockNotifyWatcher
}

var _ = gc.Suite(&stateTrackerSuite{})

func (s *stateTrackerSuite) SetUpTest(c *gc.C) {
	s.leadershipContext = &stubLeadershipContext{isLeader: true}
	s.unitTag, _ = names.ParseUnitTag("ntp/0")
}

func (s *stateTrackerSuite) TestLoadInitialStateNoRelations(c *gc.C) {
	// Green field config, no known relations, no relation status.
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{})

	r := s.newStateTracker(c)
	//No relations created.
	c.Assert(r.GetInfo(), gc.HasLen, 0)
}

func (s *stateTrackerSuite) TestLoadInitialState(c *gc.C) {
	// The state manager knows about 2 relations, 1 & 2.
	// Relation status returns 1 relation.
	// Make sure we have 1 at the end and 2 has been deleted.
	defer s.setupMocks(c).Finish()
	relTag, _ := names.ParseRelationTag("ubuntu:juju-info ntp:juju-info")
	status := []uniter.RelationStatus{{
		Tag:     relTag,
		InScope: true,
	}}
	s.expectRelationsStatus(status)
	s.expectRelation(relTag)
	s.expectRelationID(1)
	s.expectStateMgrKnownIDs([]int{1, 2})
	s.expectStateMgrRemoveRelation(2)
	s.expectStateMgrRelationFound(1)
	s.expectRelationerJoin()
	s.expectRelationSetStatusJoined()
	s.expectUnitName()
	s.expectUnitTag()
	s.expectRelationUnit()
	s.expectWatch(c)

	r := s.newStateTracker(c)

	c.Assert(r.RelationCreated(1), jc.IsTrue)
	c.Assert(r.RelationCreated(2), jc.IsFalse)
}

func (s *stateTrackerSuite) TestLoadInitialStateSuspended(c *gc.C) {
	// The state manager knows about 1 suspended relation.
	// Relation status returns 1 relation.
	// Remove known suspended out of scope relation.
	defer s.setupMocks(c).Finish()
	relTag, _ := names.ParseRelationTag("ubuntu:juju-info ntp:juju-info")
	status := []uniter.RelationStatus{{
		Tag:       relTag,
		Suspended: true,
	}}
	s.expectRelationsStatus(status)
	s.expectStateMgrKnownIDs([]int{1})
	s.expectStateMgrRemoveRelation(1)

	r := s.newStateTracker(c)

	c.Assert(r.RelationCreated(1), jc.IsFalse)
}

func (s *stateTrackerSuite) TestLoadInitialStateInScopeSuspended(c *gc.C) {
	// The state manager knows about 1 in-scope suspended relation.
	// Relation status returns 1 relation.
	defer s.setupMocks(c).Finish()
	relTag, _ := names.ParseRelationTag("ubuntu:juju-info ntp:juju-info")
	status := []uniter.RelationStatus{{
		Tag:       relTag,
		InScope:   true,
		Suspended: true,
	}}
	s.expectRelationsStatus(status)
	s.expectStateMgrKnownIDs([]int{1})
	s.expectStateMgrRelationFound(1)
	s.expectRelation(relTag)
	s.expectRelationID(1)
	s.expectUnitName()
	s.expectUnitTag()
	s.expectRelationUnit()
	s.expectWatch(c)
	s.expectRelationerJoin()
	s.expectRelationSetStatusJoined()

	r := s.newStateTracker(c)

	c.Assert(r.RelationCreated(1), jc.IsTrue)
}

func (s *stateTrackerSuite) TestLoadInitialStateKnownOnly(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{1})
	s.expectStateMgrRemoveRelation(1)

	r := s.newStateTracker(c)

	//No relations created.
	c.Assert(r.GetInfo(), gc.HasLen, 0)
}

func (s *stateTrackerSuite) TestPrepareHook(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerPrepareHook()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationJoined,
		RelationId: 1,
	}
	hookString, err := rst.PrepareHook(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookString, gc.Equals, "testing")
}

func (s *stateTrackerSuite) TestPrepareHookNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   make(map[int]relation.Relationer),
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	_, err = rst.PrepareHook(info)
	c.Assert(err, gc.ErrorMatches, "operation already executed")
}

func (s *stateTrackerSuite) TestPrepareHookOnlyRelationHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.MeterStatusChanged,
		RelationId: 1,
	}
	_, err = rst.PrepareHook(info)
	c.Assert(err, gc.ErrorMatches, "not a relation hook.*")
}

func (s *stateTrackerSuite) TestCommitHookOnlyRelationHooks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.MeterStatusChanged,
		RelationId: 1,
	}
	err = rst.CommitHook(info)
	c.Assert(err, gc.ErrorMatches, "not a relation hook.*")
}

func (s *stateTrackerSuite) TestCommitHookNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   make(map[int]relation.Relationer),
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	err = rst.CommitHook(info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateTrackerSuite) TestCommitHookRelationCreated(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHook()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	err = rst.CommitHook(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rst.RelationCreated(1), jc.IsTrue)
}

func (s *stateTrackerSuite) TestCommitHookRelationCreatedFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHookFail()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	err = rst.CommitHook(info)
	c.Assert(err, gc.NotNil)
	c.Assert(rst.RelationCreated(1), jc.IsFalse)
}

func (s *stateTrackerSuite) TestCommitHookRelationBroken(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHook()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	err = rst.CommitHook(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rst.IsKnown(1), jc.IsFalse)
}

func (s *stateTrackerSuite) TestCommitHookRelationBrokenFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHookFail()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	err = rst.CommitHook(info)
	c.Assert(err, gc.NotNil)
	c.Assert(rst.IsKnown(1), jc.IsTrue)
}

func (s *baseStateTrackerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = mocks.NewMockStateTrackerState(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.relation = mocks.NewMockRelation(ctrl)
	s.relationer = mocks.NewMockRelationer(ctrl)
	s.relationUnit = mocks.NewMockRelationUnit(ctrl)
	s.stateMgr = mocks.NewMockStateManager(ctrl)
	return ctrl
}

type syncScopesSuite struct {
	baseStateTrackerSuite

	charmDir string
}

var _ = gc.Suite(&syncScopesSuite{})

func (s *syncScopesSuite) SetUpTest(c *gc.C) {
	s.leadershipContext = &stubLeadershipContext{isLeader: true}
	s.unitTag, _ = names.ParseUnitTag("wordpress/0")
}

func (s *syncScopesSuite) setupCharmDir(c *gc.C) {
	// cleanup?
	s.charmDir = filepath.Join(c.MkDir(), "charm")
	err := os.MkdirAll(s.charmDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(s.charmDir, "metadata.yaml"), []byte(minimalMetadata), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesNoRemoteRelations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{})

	r := s.newStateTracker(c)

	remote := remotestate.Snapshot{}
	err := r.SynchronizeScopes(remote)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesNoRemoteRelationsDestroySubordinate(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{})
	s.expectUnitDestroy()

	cfg := relation.StateTrackerForTestConfig{
		St:                s.state,
		Unit:              s.unit,
		LeadershipContext: s.leadershipContext,
		StateManager:      s.stateMgr,
		Subordinate:       true,
		PrincipalName:     "ubuntu/0",
		NewRelationerFunc: func(_ relation.RelationUnit, _ relation.StateManager, _ relation.UnitGetter, _ relation.Logger) relation.Relationer {
			return s.relationer
		},
	}
	rst, err := relation.NewStateTrackerForTest(cfg)
	c.Assert(err, jc.ErrorIsNil)

	remote := remotestate.Snapshot{}
	err = rst.SynchronizeScopes(remote)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesDying(c *gc.C) {
	rst := s.testSynchronizeScopesDying(c, false)
	c.Assert(rst.IsKnown(1), jc.IsTrue)
}

func (s *syncScopesSuite) TestSynchronizeScopesDyingImplicit(c *gc.C) {
	rst := s.testSynchronizeScopesDying(c, true)
	c.Assert(rst.IsKnown(1), jc.IsFalse)
}

func (s *syncScopesSuite) testSynchronizeScopesDying(c *gc.C, implicit bool) relation.RelationStateTracker {
	// Setup
	defer s.setupMocks(c).Finish()

	rst := s.newSyncScopesStateTracker(c,
		map[int]relation.Relationer{1: s.relationer},
		map[int]string{1: ""},
	)

	// Setup for SynchronizeScopes
	s.expectRelationerRelationUnit()
	s.expectRelationUnitRelation()
	s.expectRelationUpdateSuspended(false)
	s.expectRelationerIsImplicit(implicit)

	// What the test is looking for
	s.expectRelationerSetDying()

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Dying,
				Members: map[string]int64{
					"mysql/0": 1,
				},
			},
		},
	}

	err := rst.SynchronizeScopes(remoteState)
	c.Assert(err, jc.ErrorIsNil)
	return rst
}

func (s *syncScopesSuite) TestSynchronizeScopesSuspendedDying(c *gc.C) {
	// Setup
	defer s.setupMocks(c).Finish()

	rst := s.newSyncScopesStateTracker(c,
		map[int]relation.Relationer{1: s.relationer},
		map[int]string{1: "mysql"},
	)

	// Setup for SynchronizeScopes
	s.expectRelationerRelationUnit()
	s.expectRelationUnitRelation()
	s.expectRelationUpdateSuspended(true)
	s.expectRelationerIsImplicit(true)

	// What the test is looking for
	s.expectRelationerSetDying()

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
				Members: map[string]int64{
					"mysql/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"mysql": 1,
				},
			},
		},
	}

	err := rst.SynchronizeScopes(remoteState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rst.IsKnown(1), jc.IsFalse)
}

func (s *syncScopesSuite) TestSynchronizeScopesJoinRelation(c *gc.C) {
	// wordpress unit with mysql relation
	s.setupCharmDir(c)
	defer s.setupMocks(c).Finish()
	// Setup for SynchronizeScopes()
	s.expectRelationById(1)
	ep := &uniter.Endpoint{
		charm.Relation{
			Role:      charm.RoleRequirer,
			Name:      "mysql",
			Interface: "db",
			Scope:     charm.ScopeGlobal,
		}}
	s.expectRelationEndpoint(ep)
	s.expectRelationUnit()
	s.expectRelationOtherApplication()

	// Setup for joinRelation()
	s.expectUnitName()
	s.expectUnitTag()
	s.expectWatch(c)
	s.expectRelationerJoin()
	s.expectRelationSetStatusJoined()
	s.expectRelationID(1)

	rst := s.newSyncScopesStateTracker(c,
		make(map[int]relation.Relationer),
		make(map[int]string),
	)

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"mysql/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"mysql": 1,
				},
			},
		},
	}

	err := rst.SynchronizeScopes(remoteState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rst.RemoteApplication(1), gc.Equals, "mysql")
}

func (s *syncScopesSuite) assertSynchronizeScopesFailImplementedBy(c *gc.C, createCharmDir bool) {
	if createCharmDir {
		// wordpress unit with mysql relation
		s.setupCharmDir(c)
	}
	defer s.setupMocks(c).Finish()
	// Setup for SynchronizeScopes()
	s.expectRelationById(1)
	ep := &uniter.Endpoint{
		charm.Relation{
			// changing to RoleProvider will cause ImplementedBy to fail.
			Role:      charm.RoleProvider,
			Name:      "mysql",
			Interface: "db",
			Scope:     charm.ScopeGlobal,
		}}
	s.expectRelationOtherApplication()
	s.expectRelationEndpoint(ep)
	s.expectString()

	rst := s.newSyncScopesStateTracker(c,
		make(map[int]relation.Relationer),
		make(map[int]string),
	)

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"mysql/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"mysql": 1,
				},
			},
		},
	}

	err := rst.SynchronizeScopes(remoteState)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesFailImplementedBy(c *gc.C) {
	s.assertSynchronizeScopesFailImplementedBy(c, true)
}

func (s *syncScopesSuite) TestSynchronizeScopesIgnoresMissingCharmDir(c *gc.C) {
	s.assertSynchronizeScopesFailImplementedBy(c, false)
}

func (s *syncScopesSuite) TestSynchronizeScopesSeenNotDying(c *gc.C) {
	// Setup
	defer s.setupMocks(c).Finish()

	rst := s.newSyncScopesStateTracker(c,
		map[int]relation.Relationer{1: s.relationer},
		map[int]string{1: "mysql"},
	)

	// Setup for SynchronizeScopes
	s.expectRelationerRelationUnit()
	s.expectRelationUnitRelation()
	s.expectRelationUpdateSuspended(false)

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"mysql/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"mysql": 1,
				},
			},
		},
	}

	err := rst.SynchronizeScopes(remoteState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rst.RemoteApplication(1), gc.Equals, "mysql")
}

// Relationer
func (s *baseStateTrackerSuite) expectRelationerPrepareHook() {
	s.relationer.EXPECT().PrepareHook(gomock.Any()).Return("testing", nil)
}

func (s *baseStateTrackerSuite) expectRelationerCommitHook() {
	s.relationer.EXPECT().CommitHook(gomock.Any()).Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationerCommitHookFail() {
	s.relationer.EXPECT().CommitHook(gomock.Any()).Return(errors.NotFoundf("testing"))
}

func (s *baseStateTrackerSuite) expectRelationerJoin() {
	s.relationer.EXPECT().Join().Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationerRelationUnit() {
	s.relationer.EXPECT().RelationUnit().Return(s.relationUnit)
}

func (s *baseStateTrackerSuite) expectRelationerSetDying() {
	s.relationer.EXPECT().SetDying().Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationerIsImplicit(imp bool) {
	s.relationer.EXPECT().IsImplicit().Return(imp)
}

// RelationUnit
func (s *baseStateTrackerSuite) expectRelationUnitRelation() {
	s.relationUnit.EXPECT().Relation().Return(s.relation)
}

// Relation
func (s *baseStateTrackerSuite) expectRelationUpdateSuspended(suspend bool) {
	s.relation.EXPECT().UpdateSuspended(suspend)
}

func (s *baseStateTrackerSuite) expectRelationUnit() {
	s.relation.EXPECT().Unit(s.unitTag).Return(s.relationUnit, nil).AnyTimes()
}

func (s *baseStateTrackerSuite) expectRelationSetStatusJoined() {
	s.relation.EXPECT().SetStatus(corerelation.Joined)
}

func (s *baseStateTrackerSuite) expectRelationID(id int) {
	s.relation.EXPECT().Id().Return(id).AnyTimes()
}

func (s *baseStateTrackerSuite) expectRelationEndpoint(ep *uniter.Endpoint) {
	s.relation.EXPECT().Endpoint().Return(ep, nil)
}

func (s *syncScopesSuite) expectRelationOtherApplication() {
	s.relation.EXPECT().OtherApplication().Return("mysql")
}

func (s *syncScopesSuite) expectString() {
	s.relation.EXPECT().String().Return("test me").AnyTimes()
}

// StateManager
func (s *baseStateTrackerSuite) expectStateMgrRemoveRelation(id int) {
	s.stateMgr.EXPECT().RemoveRelation(id, s.state, map[string]bool{}).Return(nil)
}

func (s *baseStateTrackerSuite) expectStateMgrKnownIDs(ids []int) {
	s.stateMgr.EXPECT().KnownIDs().Return(ids)
}

func (s *baseStateTrackerSuite) expectStateMgrRelationFound(id int) {
	s.stateMgr.EXPECT().RelationFound(id).Return(true)
}

// State
func (s *baseStateTrackerSuite) expectRelation(relTag names.RelationTag) {
	s.state.EXPECT().Relation(relTag).Return(s.relation, nil)
}

func (s *syncScopesSuite) expectRelationById(id int) {
	s.state.EXPECT().RelationById(id).Return(s.relation, nil)
}

// Unit
func (s *baseStateTrackerSuite) expectUnitTag() {
	s.unit.EXPECT().Tag().Return(s.unitTag)
}

func (s *baseStateTrackerSuite) expectUnitName() {
	s.unit.EXPECT().Name().Return(s.unitTag.Id())
}

func (s *baseStateTrackerSuite) expectUnitDestroy() {
	s.unit.EXPECT().Destroy().Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationsStatusEmpty() {
	s.unit.EXPECT().RelationsStatus().Return([]uniter.RelationStatus{}, nil)
}

func (s *baseStateTrackerSuite) expectRelationsStatus(status []uniter.RelationStatus) {
	s.unit.EXPECT().RelationsStatus().Return(status, nil)
}

func (s *baseStateTrackerSuite) expectWatch(c *gc.C) {
	do := func() {
		go func() {
			select {
			case s.unitChanges <- struct{}{}:
			case <-time.After(coretesting.LongWait):
				c.Fatal("timed out unit change")
			}
		}()
	}
	s.unitChanges = make(chan struct{})
	s.watcher = watchertest.NewMockNotifyWatcher(s.unitChanges)
	s.unit.EXPECT().Watch().Return(s.watcher, nil).Do(do)
}

func (s *baseStateTrackerSuite) newStateTracker(c *gc.C) relation.RelationStateTracker {
	cfg := relation.StateTrackerForTestConfig{
		St:                s.state,
		Unit:              s.unit,
		LeadershipContext: s.leadershipContext,
		StateManager:      s.stateMgr,
		NewRelationerFunc: func(_ relation.RelationUnit, _ relation.StateManager, _ relation.UnitGetter, _ relation.Logger) relation.Relationer {
			return s.relationer
		},
	}
	rst, err := relation.NewStateTrackerForTest(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return rst
}

func (s *syncScopesSuite) newSyncScopesStateTracker(c *gc.C, relationers map[int]relation.Relationer, appNames map[int]string) relation.RelationStateTracker {
	cfg := relation.StateTrackerForTestConfig{
		St:                s.state,
		Unit:              s.unit,
		LeadershipContext: s.leadershipContext,
		StateManager:      s.stateMgr,
		NewRelationerFunc: func(_ relation.RelationUnit, _ relation.StateManager, _ relation.UnitGetter, _ relation.Logger) relation.Relationer {
			return s.relationer
		},
		Relationers:   relationers,
		RemoteAppName: appNames,
		CharmDir:      s.charmDir,
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return rst
}
