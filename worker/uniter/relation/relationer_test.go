// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"fmt"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/relation/mocks"
)

type relationerSuite struct {
	stateManager *mocks.MockStateManager
	relationUnit *mocks.MockRelationUnit
	relation     *mocks.MockRelation
	unitGetter   *mocks.MockUnitGetter
}

var _ = gc.Suite(&relationerSuite{})

func (s *relationerSuite) TestImplicitRelationerPrepareHook(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(implicitRelationEndpoint())

	r := s.newRelationer()

	// Hooks are not allowed.
	_, err := r.PrepareHook(hook.Info{})
	c.Assert(err, gc.ErrorMatches, `restart immediately`)
}

func (s *relationerSuite) TestImplicitRelationerCommitHook(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(implicitRelationEndpoint())

	r := s.newRelationer()

	// Hooks are not allowed.
	err := r.CommitHook(hook.Info{})
	c.Assert(err, gc.ErrorMatches, `restart immediately`)
}

func (s *relationerSuite) TestImplicitRelationerSetDying(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(implicitRelationEndpoint())
	s.expectLeaveScope()
	s.expectRemoveRelation()

	r := s.newRelationer()

	// Set it to Dying
	c.Assert(r.IsDying(), jc.IsFalse)
	err := r.SetDying()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.IsDying(), jc.IsTrue)
}

func (s *relationerSuite) TestSetDying(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())

	r := s.newRelationer()

	// Set it to Dying
	c.Assert(r.IsDying(), jc.IsFalse)
	err := r.SetDying()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.IsDying(), jc.IsTrue)
}

func (s *relationerSuite) TestIfDyingFailJoin(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())

	r := s.newRelationer()

	// Set it to Dying
	err := r.SetDying()
	c.Assert(err, jc.ErrorIsNil)

	// Try to Join
	err = r.Join()
	c.Assert(err, gc.ErrorMatches, `dying relationer must not join!`)
}

func (s *relationerSuite) TestCommitHookRelationBrokenDies(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectLeaveScope()
	s.expectRemoveRelation()

	r := s.newRelationer()

	err := r.CommitHook(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationerSuite) TestCommitHookRelationRemoved(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.relationUnit.EXPECT().LeaveScope().Return(&params.Error{Code: "not found"})
	s.expectRemoveRelation()

	r := s.newRelationer()

	err := r.CommitHook(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationerSuite) TestCommitHook(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelation(nil)
	s.expectSetRelation()

	r := s.newRelationer()

	err := r.CommitHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationerSuite) TestCommitHookRelationFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelation(errors.NotImplementedf("testing"))

	r := s.newRelationer()

	err := r.CommitHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *relationerSuite) TestPrepareHookRelationFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelation(errors.NotImplementedf("testing"))

	r := s.newRelationer()

	_, err := r.PrepareHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *relationerSuite) TestPrepareHookValidateFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelationFailValidate()

	r := s.newRelationer()

	// relationID and state id being different will fail validation.
	name, err := r.PrepareHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, gc.NotNil)
	c.Assert(name, gc.Equals, "")
}

func (s *relationerSuite) TestPrepareHook(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	ep := endpoint()
	s.expectEndpoint(ep)
	s.expectEndpoint(ep)
	s.expectStateManagerRelation(nil)

	r := s.newRelationer()

	name, err := r.PrepareHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, fmt.Sprintf("%s-%s", ep.Name, hooks.RelationJoined))
}

func (s *relationerSuite) TestJoinRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEnterScope()
	s.expectRelationFound(true)

	r := s.newRelationer()

	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationerSuite) TestJoinRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEnterScope()
	s.expectRelationFound(false)
	s.expectSetRelation()

	r := s.newRelationer()
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationerSuite) newRelationer() relation.Relationer {
	logger := loggo.GetLogger("test")
	return relation.NewRelationer(s.relationUnit, s.stateManager, s.unitGetter, logger)
}

func (s *relationerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateManager = mocks.NewMockStateManager(ctrl)
	s.relationUnit = mocks.NewMockRelationUnit(ctrl)
	s.relation = mocks.NewMockRelation(ctrl)
	s.unitGetter = mocks.NewMockUnitGetter(ctrl)
	// Setup for NewRelationer
	s.expectRelationUnitRelation()
	s.expectRelationId()
	return ctrl
}

func implicitRelationEndpoint() apiuniter.Endpoint {
	return apiuniter.Endpoint{
		charm.Relation{
			Role:      charm.RoleProvider,
			Name:      "juju-info",
			Interface: "juju-info",
		}}
}

func endpoint() apiuniter.Endpoint {
	return apiuniter.Endpoint{
		charm.Relation{
			Role:      charm.RoleRequirer,
			Name:      "mysql",
			Interface: "db",
			Scope:     charm.ScopeGlobal,
		}}
}

// RelationUnit
func (s *relationerSuite) expectEndpoint(ep apiuniter.Endpoint) {
	s.relationUnit.EXPECT().Endpoint().Return(ep)
}

func (s *relationerSuite) expectLeaveScope() {
	s.relationUnit.EXPECT().LeaveScope().Return(nil)
}

func (s *relationerSuite) expectEnterScope() {
	s.relationUnit.EXPECT().EnterScope().Return(nil)
}

func (s *relationerSuite) expectRelationUnitRelation() {
	s.relationUnit.EXPECT().Relation().Return(s.relation)
}

// Relation
func (s *relationerSuite) expectRelationId() {
	s.relation.EXPECT().Id().Return(1)
}

// StateManager
func (s *relationerSuite) expectRemoveRelation() {
	s.stateManager.EXPECT().RemoveRelation(1, s.unitGetter, map[string]bool{}).Return(nil)
}

func (s *relationerSuite) expectRelationFound(found bool) {
	s.stateManager.EXPECT().RelationFound(1).Return(found)
}

func (s *relationerSuite) expectSetRelation() {
	s.stateManager.EXPECT().SetRelation(gomock.Any()).Return(nil)
}

func (s *relationerSuite) expectStateManagerRelation(err error) {
	st := relation.NewState(1)
	s.stateManager.EXPECT().Relation(1).Return(st, err)
}

func (s *relationerSuite) expectStateManagerRelationFailValidate() {
	st := relation.NewState(0)
	s.stateManager.EXPECT().Relation(1).Return(st, nil)
}
