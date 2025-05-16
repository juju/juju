// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	stdcontext "context"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/relation"
	"github.com/juju/juju/internal/worker/uniter/relation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

type stateTrackerSuite struct {
	baseStateTrackerSuite
}

type baseStateTrackerSuite struct {
	leadershipContext context.LeadershipContext
	unitTag           names.UnitTag
	unitChanges       chan struct{}

	client       *mocks.MockStateTrackerClient
	unit         *api.MockUnit
	relation     *api.MockRelation
	relationer   *mocks.MockRelationer
	relationUnit *api.MockRelationUnit
	stateMgr     *mocks.MockStateManager
	watcher      *watchertest.MockNotifyWatcher
}

func TestStateTrackerSuite(t *stdtesting.T) { tc.Run(t, &stateTrackerSuite{}) }
func (s *stateTrackerSuite) SetUpTest(c *tc.C) {
	s.leadershipContext = &stubLeadershipContext{isLeader: true}
	s.unitTag, _ = names.ParseUnitTag("ntp/0")
}

func (s *stateTrackerSuite) TestLoadInitialStateNoRelations(c *tc.C) {
	// Green field config, no known relations, no relation status.
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{})

	r := s.newStateTracker(c)
	//No relations created.
	c.Assert(r.GetInfo(), tc.HasLen, 0)
}

func (s *stateTrackerSuite) TestLoadInitialState(c *tc.C) {
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

	c.Assert(r.RelationCreated(1), tc.IsTrue)
	c.Assert(r.RelationCreated(2), tc.IsFalse)
}

func (s *stateTrackerSuite) TestLoadInitialStateSuspended(c *tc.C) {
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

	c.Assert(r.RelationCreated(1), tc.IsFalse)
}

func (s *stateTrackerSuite) TestLoadInitialStateInScopeSuspended(c *tc.C) {
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

	c.Assert(r.RelationCreated(1), tc.IsTrue)
}

func (s *stateTrackerSuite) TestLoadInitialStateKnownOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{1})
	s.expectStateMgrRemoveRelation(1)

	r := s.newStateTracker(c)

	//No relations created.
	c.Assert(r.GetInfo(), tc.HasLen, 0)
}

func (s *stateTrackerSuite) TestPrepareHook(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerPrepareHook()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationJoined,
		RelationId: 1,
	}
	hookString, err := rst.PrepareHook(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hookString, tc.Equals, "testing")
}

func (s *stateTrackerSuite) TestPrepareHookNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   make(map[int]relation.Relationer),
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	_, err = rst.PrepareHook(info)
	c.Assert(err, tc.ErrorMatches, "operation already executed")
}

func (s *stateTrackerSuite) TestPrepareHookOnlyRelationHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.PebbleCustomNotice,
		RelationId: 1,
	}
	_, err = rst.PrepareHook(info)
	c.Assert(err, tc.ErrorMatches, "not a relation hook.*")
}

func (s *stateTrackerSuite) TestCommitHookOnlyRelationHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.PebbleCustomNotice,
		RelationId: 1,
	}
	err = rst.CommitHook(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, "not a relation hook.*")
}

func (s *stateTrackerSuite) TestCommitHookNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   make(map[int]relation.Relationer),
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	err = rst.CommitHook(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateTrackerSuite) TestCommitHookRelationCreated(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHook()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	err = rst.CommitHook(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rst.RelationCreated(1), tc.IsTrue)
}

func (s *stateTrackerSuite) TestCommitHookRelationCreatedFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHookFail()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationCreated,
		RelationId: 1,
	}
	err = rst.CommitHook(c.Context(), info)
	c.Assert(err, tc.NotNil)
	c.Assert(rst.RelationCreated(1), tc.IsFalse)
}

func (s *stateTrackerSuite) TestCommitHookRelationBroken(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHook()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	err = rst.CommitHook(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rst.IsKnown(1), tc.IsFalse)
}

func (s *stateTrackerSuite) TestCommitHookRelationBrokenFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationerCommitHookFail()
	cfg := relation.StateTrackerForTestConfig{
		Relationers:   map[int]relation.Relationer{1: s.relationer},
		RemoteAppName: make(map[int]string),
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	err = rst.CommitHook(c.Context(), info)
	c.Assert(err, tc.NotNil)
	c.Assert(rst.IsKnown(1), tc.IsTrue)
}

func (s *baseStateTrackerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockStateTrackerClient(ctrl)
	s.unit = api.NewMockUnit(ctrl)
	s.relation = api.NewMockRelation(ctrl)
	s.relationer = mocks.NewMockRelationer(ctrl)
	s.relationUnit = api.NewMockRelationUnit(ctrl)
	s.stateMgr = mocks.NewMockStateManager(ctrl)

	s.relation.EXPECT().String().AnyTimes()

	return ctrl
}

type syncScopesSuite struct {
	baseStateTrackerSuite

	charmDir string
}

func TestSyncScopesSuite(t *stdtesting.T) { tc.Run(t, &syncScopesSuite{}) }
func (s *syncScopesSuite) SetUpTest(c *tc.C) {
	s.leadershipContext = &stubLeadershipContext{isLeader: true}
	s.unitTag, _ = names.ParseUnitTag("wordpress/0")
}

func (s *syncScopesSuite) setupCharmDir(c *tc.C) {
	// cleanup?
	s.charmDir = filepath.Join(c.MkDir(), "charm")
	err := os.MkdirAll(s.charmDir, 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(s.charmDir, "metadata.yaml"), []byte(minimalMetadata), 0755)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesNoRemoteRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{})

	r := s.newStateTracker(c)

	remote := remotestate.Snapshot{}
	err := r.SynchronizeScopes(c.Context(), remote)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesNoRemoteRelationsDestroySubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRelationsStatusEmpty()
	s.expectStateMgrKnownIDs([]int{})
	s.expectUnitDestroy()

	cfg := relation.StateTrackerForTestConfig{
		Client:            s.client,
		Unit:              s.unit,
		LeadershipContext: s.leadershipContext,
		StateManager:      s.stateMgr,
		Subordinate:       true,
		PrincipalName:     "ubuntu/0",
		NewRelationerFunc: func(_ api.RelationUnit, _ relation.StateManager, _ relation.UnitGetter, _ logger.Logger) relation.Relationer {
			return s.relationer
		},
	}
	rst, err := relation.NewStateTrackerForTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)

	remote := remotestate.Snapshot{}
	err = rst.SynchronizeScopes(c.Context(), remote)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesDying(c *tc.C) {
	rst := s.testSynchronizeScopesDying(c, false)
	c.Assert(rst.IsKnown(1), tc.IsTrue)
}

func (s *syncScopesSuite) TestSynchronizeScopesDyingImplicit(c *tc.C) {
	rst := s.testSynchronizeScopesDying(c, true)
	c.Assert(rst.IsKnown(1), tc.IsFalse)
}

func (s *syncScopesSuite) testSynchronizeScopesDying(c *tc.C, implicit bool) relation.RelationStateTracker {
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

	err := rst.SynchronizeScopes(c.Context(), remoteState)
	c.Assert(err, tc.ErrorIsNil)
	return rst
}

func (s *syncScopesSuite) TestSynchronizeScopesSuspendedDying(c *tc.C) {
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

	err := rst.SynchronizeScopes(c.Context(), remoteState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rst.IsKnown(1), tc.IsFalse)
}

func (s *syncScopesSuite) TestSynchronizeScopesJoinRelation(c *tc.C) {
	// wordpress unit with mysql relation
	s.setupCharmDir(c)
	defer s.setupMocks(c).Finish()
	// Setup for SynchronizeScopes()
	s.expectRelationById(1)
	ep := &uniter.Endpoint{
		Relation: charm.Relation{
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

	err := rst.SynchronizeScopes(c.Context(), remoteState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rst.RemoteApplication(1), tc.Equals, "mysql")
}

func (s *syncScopesSuite) TestSynchronizeScopesFailImplementedBy(c *tc.C) {
	// wordpress unit with mysql relation
	s.setupCharmDir(c)
	defer s.setupMocks(c).Finish()
	// Setup for SynchronizeScopes()
	s.expectRelationById(1)
	ep := &uniter.Endpoint{
		Relation: charm.Relation{
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

	err := rst.SynchronizeScopes(c.Context(), remoteState)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *syncScopesSuite) TestSynchronizeScopesIgnoresMissingCharmDir(c *tc.C) {
	s.charmDir = c.MkDir()
	err := os.Remove(s.charmDir)
	c.Assert(err, tc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	// Setup for SynchronizeScopes()
	s.expectRelationById(1)
	ep := &uniter.Endpoint{
		Relation: charm.Relation{
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

	err = rst.SynchronizeScopes(c.Context(), remoteState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rst.RemoteApplication(1), tc.Equals, "mysql")
}

func (s *syncScopesSuite) TestSynchronizeScopesSeenNotDying(c *tc.C) {
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

	err := rst.SynchronizeScopes(c.Context(), remoteState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rst.RemoteApplication(1), tc.Equals, "mysql")
}

// Relationer
func (s *baseStateTrackerSuite) expectRelationerPrepareHook() {
	s.relationer.EXPECT().PrepareHook(gomock.Any()).Return("testing", nil)
}

func (s *baseStateTrackerSuite) expectRelationerCommitHook() {
	s.relationer.EXPECT().CommitHook(gomock.Any(), gomock.Any()).Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationerCommitHookFail() {
	s.relationer.EXPECT().CommitHook(gomock.Any(), gomock.Any()).Return(errors.NotFoundf("testing"))
}

func (s *baseStateTrackerSuite) expectRelationerJoin() {
	s.relationer.EXPECT().Join(gomock.Any()).Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationerRelationUnit() {
	s.relationer.EXPECT().RelationUnit().Return(s.relationUnit)
}

func (s *baseStateTrackerSuite) expectRelationerSetDying() {
	s.relationer.EXPECT().SetDying(gomock.Any()).Return(nil)
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
	s.relation.EXPECT().Unit(gomock.Any(), s.unitTag).Return(s.relationUnit, nil).AnyTimes()
}

func (s *baseStateTrackerSuite) expectRelationSetStatusJoined() {
	s.relation.EXPECT().SetStatus(gomock.Any(), corerelation.Joined)
}

func (s *baseStateTrackerSuite) expectRelationID(id int) {
	s.relation.EXPECT().Id().Return(id).AnyTimes()
}

func (s *baseStateTrackerSuite) expectRelationEndpoint(ep *uniter.Endpoint) {
	s.relation.EXPECT().Endpoint(gomock.Any()).Return(ep, nil)
}

func (s *syncScopesSuite) expectRelationOtherApplication() {
	s.relation.EXPECT().OtherApplication().Return("mysql")
}

func (s *syncScopesSuite) expectString() {
	s.relation.EXPECT().String().Return("test me").AnyTimes()
}

// StateManager
func (s *baseStateTrackerSuite) expectStateMgrRemoveRelation(id int) {
	s.stateMgr.EXPECT().RemoveRelation(gomock.Any(), id, s.client, map[string]bool{}).Return(nil)
}

func (s *baseStateTrackerSuite) expectStateMgrKnownIDs(ids []int) {
	s.stateMgr.EXPECT().KnownIDs().Return(ids)
}

func (s *baseStateTrackerSuite) expectStateMgrRelationFound(id int) {
	s.stateMgr.EXPECT().RelationFound(id).Return(true)
}

// State
func (s *baseStateTrackerSuite) expectRelation(relTag names.RelationTag) {
	s.client.EXPECT().Relation(gomock.Any(), relTag).Return(s.relation, nil)
}

func (s *syncScopesSuite) expectRelationById(id int) {
	s.client.EXPECT().RelationById(gomock.Any(), id).Return(s.relation, nil)
	s.relation.EXPECT().String().AnyTimes()
}

// Unit
func (s *baseStateTrackerSuite) expectUnitTag() {
	s.unit.EXPECT().Tag().Return(s.unitTag)
}

func (s *baseStateTrackerSuite) expectUnitName() {
	s.unit.EXPECT().Name().Return(s.unitTag.Id())
}

func (s *baseStateTrackerSuite) expectUnitDestroy() {
	s.unit.EXPECT().Destroy(gomock.Any()).Return(nil)
}

func (s *baseStateTrackerSuite) expectRelationsStatusEmpty() {
	s.unit.EXPECT().RelationsStatus(gomock.Any()).Return([]uniter.RelationStatus{}, nil)
}

func (s *baseStateTrackerSuite) expectRelationsStatus(status []uniter.RelationStatus) {
	s.unit.EXPECT().RelationsStatus(gomock.Any()).Return(status, nil)
}

func (s *baseStateTrackerSuite) expectWatch(c *tc.C) {
	s.unitChanges = make(chan struct{})
	s.watcher = watchertest.NewMockNotifyWatcher(s.unitChanges)
	s.unit.EXPECT().Watch(gomock.Any()).DoAndReturn(func(stdcontext.Context) (watcher.Watcher[struct{}], error) {
		go func() {
			select {
			case s.unitChanges <- struct{}{}:
			case <-time.After(coretesting.LongWait):
				c.Fatal("timed out unit change")
			}
		}()
		return s.watcher, nil
	})
}

func (s *baseStateTrackerSuite) newStateTracker(c *tc.C) relation.RelationStateTracker {
	cfg := relation.StateTrackerForTestConfig{
		Client:            s.client,
		Unit:              s.unit,
		LeadershipContext: s.leadershipContext,
		StateManager:      s.stateMgr,
		NewRelationerFunc: func(_ api.RelationUnit, _ relation.StateManager, _ relation.UnitGetter, _ logger.Logger) relation.Relationer {
			return s.relationer
		},
	}
	rst, err := relation.NewStateTrackerForTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)
	return rst
}

func (s *syncScopesSuite) newSyncScopesStateTracker(c *tc.C, relationers map[int]relation.Relationer, appNames map[int]string) relation.RelationStateTracker {
	cfg := relation.StateTrackerForTestConfig{
		Client:            s.client,
		Unit:              s.unit,
		LeadershipContext: s.leadershipContext,
		StateManager:      s.stateMgr,
		NewRelationerFunc: func(_ api.RelationUnit, _ relation.StateManager, _ relation.UnitGetter, _ logger.Logger) relation.Relationer {
			return s.relationer
		},
		Relationers:   relationers,
		RemoteAppName: appNames,
		CharmDir:      s.charmDir,
	}
	rst, err := relation.NewStateTrackerForSyncScopesTest(c, cfg)
	c.Assert(err, tc.ErrorIsNil)
	return rst
}
