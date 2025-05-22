// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/relation"
	"github.com/juju/juju/internal/worker/uniter/relation/mocks"
	"github.com/juju/juju/rpc/params"
)

type relationerSuite struct {
	stateManager *mocks.MockStateManager
	relationUnit *api.MockRelationUnit
	relation     *api.MockRelation
	unitGetter   *mocks.MockUnitGetter
}

func TestRelationerSuite(t *testing.T) {
	tc.Run(t, &relationerSuite{})
}

func (s *relationerSuite) TestImplicitRelationerPrepareHook(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(implicitRelationEndpoint())

	r := s.newRelationer(c)

	// Hooks are not allowed.
	_, err := r.PrepareHook(hook.Info{})
	c.Assert(err, tc.ErrorMatches, `restart immediately`)
}

func (s *relationerSuite) TestImplicitRelationerCommitHook(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(implicitRelationEndpoint())

	r := s.newRelationer(c)

	// Hooks are not allowed.
	err := r.CommitHook(c.Context(), hook.Info{})
	c.Assert(err, tc.ErrorMatches, `restart immediately`)
}

func (s *relationerSuite) TestImplicitRelationerSetDying(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(implicitRelationEndpoint())
	s.expectLeaveScope()
	s.expectRemoveRelation()

	r := s.newRelationer(c)

	// Set it to Dying
	c.Assert(r.IsDying(), tc.IsFalse)
	err := r.SetDying(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.IsDying(), tc.IsTrue)
}

func (s *relationerSuite) TestSetDying(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())

	r := s.newRelationer(c)

	// Set it to Dying
	c.Assert(r.IsDying(), tc.IsFalse)
	err := r.SetDying(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.IsDying(), tc.IsTrue)
}

func (s *relationerSuite) TestIfDyingFailJoin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())

	r := s.newRelationer(c)

	// Set it to Dying
	err := r.SetDying(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Try to Join
	err = r.Join(c.Context())
	c.Assert(err, tc.ErrorMatches, `dying relationer must not join!`)
}

func (s *relationerSuite) TestCommitHookRelationBrokenDies(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectLeaveScope()
	s.expectRemoveRelation()

	r := s.newRelationer(c)

	err := r.CommitHook(c.Context(), hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationerSuite) TestCommitHookRelationRemoved(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.relationUnit.EXPECT().LeaveScope(gomock.Any()).Return(&params.Error{Code: "not found"})
	s.expectRemoveRelation()

	r := s.newRelationer(c)

	err := r.CommitHook(c.Context(), hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationerSuite) TestCommitHook(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelation(nil)
	s.expectSetRelation()

	r := s.newRelationer(c)

	err := r.CommitHook(c.Context(), hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationerSuite) TestCommitHookRelationFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelation(errors.NotImplementedf("testing"))

	r := s.newRelationer(c)

	err := r.CommitHook(c.Context(), hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *relationerSuite) TestPrepareHookRelationFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelation(errors.NotImplementedf("testing"))

	r := s.newRelationer(c)

	_, err := r.PrepareHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func (s *relationerSuite) TestPrepareHookValidateFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEndpoint(endpoint())
	s.expectStateManagerRelationFailValidate()

	r := s.newRelationer(c)

	// relationID and state id being different will fail validation.
	name, err := r.PrepareHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, tc.NotNil)
	c.Assert(name, tc.Equals, "")
}

func (s *relationerSuite) TestPrepareHook(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	ep := endpoint()
	s.expectEndpoint(ep)
	s.expectEndpoint(ep)
	s.expectStateManagerRelation(nil)

	r := s.newRelationer(c)

	name, err := r.PrepareHook(hook.Info{Kind: hooks.RelationJoined, RelationId: 1})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, fmt.Sprintf("%s-%s", ep.Name, hooks.RelationJoined))
}

func (s *relationerSuite) TestJoinRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEnterScope()
	s.expectRelationFound(true)

	r := s.newRelationer(c)

	err := r.Join(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationerSuite) TestJoinRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Setup for test
	s.expectEnterScope()
	s.expectRelationFound(false)
	s.expectSetRelation()

	r := s.newRelationer(c)
	err := r.Join(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationerSuite) newRelationer(c *tc.C) relation.Relationer {
	logger := loggertesting.WrapCheckLog(c)
	return relation.NewRelationer(s.relationUnit, s.stateManager, s.unitGetter, logger)
}

func (s *relationerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateManager = mocks.NewMockStateManager(ctrl)
	s.relationUnit = api.NewMockRelationUnit(ctrl)
	s.relation = api.NewMockRelation(ctrl)
	s.unitGetter = mocks.NewMockUnitGetter(ctrl)
	// Setup for NewRelationer
	s.expectRelationUnitRelation()
	s.expectRelationId()
	return ctrl
}

func implicitRelationEndpoint() apiuniter.Endpoint {
	return apiuniter.Endpoint{
		Relation: charm.Relation{
			Role:      charm.RoleProvider,
			Name:      corerelation.JujuInfo,
			Interface: corerelation.JujuInfo,
		}}
}

func endpoint() apiuniter.Endpoint {
	return apiuniter.Endpoint{
		Relation: charm.Relation{
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
	s.relationUnit.EXPECT().LeaveScope(gomock.Any()).Return(nil)
}

func (s *relationerSuite) expectEnterScope() {
	s.relationUnit.EXPECT().EnterScope(gomock.Any()).Return(nil)
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
	s.stateManager.EXPECT().RemoveRelation(gomock.Any(), 1, s.unitGetter, map[string]bool{}).Return(nil)
}

func (s *relationerSuite) expectRelationFound(found bool) {
	s.stateManager.EXPECT().RelationFound(1).Return(found)
}

func (s *relationerSuite) expectSetRelation() {
	s.stateManager.EXPECT().SetRelation(gomock.Any(), gomock.Any()).Return(nil)
}

func (s *relationerSuite) expectStateManagerRelation(err error) {
	st := relation.NewState(1)
	s.stateManager.EXPECT().Relation(1).Return(st, err)
}

func (s *relationerSuite) expectStateManagerRelationFailValidate() {
	st := relation.NewState(0)
	s.stateManager.EXPECT().Relation(1).Return(st, nil)
}
