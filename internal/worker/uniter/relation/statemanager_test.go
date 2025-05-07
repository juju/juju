// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/relation"
	relmocks "github.com/juju/juju/internal/worker/uniter/relation/mocks"
	"github.com/juju/juju/rpc/params"
)

type stateManagerSuite struct {
	mockUnitRW     *mocks.MockUnitStateReadWriter
	mockUnitGetter *relmocks.MockUnitGetter
}

func (s *stateManagerSuite) TestNewStateManagerHasState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	for _, st := range states {
		v, err := mgr.Relation(st.RelationId)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*v, tc.DeepEquals, st)
	}
}

func (s *stateManagerSuite) TestNewStateManagerNoState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateEmpty()

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgr.KnownIDs(), tc.HasLen, 0)
}

func (s *stateManagerSuite) TestNewStateManagerErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateEmptyError()

	_, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *stateManagerSuite) TestKnownIds(c *tc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	ids := mgr.KnownIDs()
	intSet := set.NewInts(ids...)
	c.Assert(intSet.Size(), tc.Equals, 4, tc.Commentf("obtained %v", intSet.Values()))
	for _, exp := range states {
		c.Assert(intSet.Contains(exp.RelationId), jc.IsTrue)
	}
}

func (s *stateManagerSuite) TestRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	st, err := mgr.Relation(states[1].RelationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*st, tc.DeepEquals, states[1])
}

func (s *stateManagerSuite) TestRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	_ = s.setupFourStates(c)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	_, err = mgr.Relation(42)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateManagerSuite) TestSetNew(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateEmpty()
	st2 := &relation.State{RelationId: 456}
	st2.Members = map[string]int64{
		"bar/0": 3,
		"bar/1": 4,
	}
	s.expectSetState(c, *st2)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.SetRelation(context.Background(), st2)
	c.Assert(err, jc.ErrorIsNil)
	found := mgr.RelationFound(456)
	c.Assert(found, jc.IsTrue)
}

func (s *stateManagerSuite) TestSetChangeExisting(c *tc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	states[3].ChangedPending = "foo/1"
	s.expectSetState(c, states...)
	st := states[3]

	err = mgr.SetRelation(context.Background(), &st)
	c.Assert(err, jc.ErrorIsNil)

	obtained, err := mgr.Relation(st.RelationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtained, tc.DeepEquals, st)
}

func (s *stateManagerSuite) TestSetChangeExistingFail(c *tc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)
	s.expectSetStateError()

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	st := states[3]
	st.ChangedPending = "foo/1"
	err = mgr.SetRelation(context.Background(), &st)
	c.Assert(err, jc.ErrorIs, errors.BadRequest)

	obtained, err := mgr.Relation(st.RelationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtained, tc.DeepEquals, states[3])
}

func (s *stateManagerSuite) TestRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()
	state := relation.State{RelationId: 1}
	s.expectState(c, state)
	s.expectSetStateEmpty(c)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(context.Background(), 1, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateManagerSuite) TestRemoveNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(context.Background(), 1, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateManagerSuite) TestRemoveFailHasMembers(c *tc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo)
	s.mockUnitGetter.EXPECT().Unit(gomock.Any(), names.NewUnitTag("foo/1")).Return(nil, nil)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(context.Background(), 99, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, tc.ErrorMatches, `*has members: \[foo/1\]`)
}

func (s *stateManagerSuite) TestRemoveIgnoresMissingUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo)
	s.expectSetStateEmpty(c)
	s.mockUnitGetter.EXPECT().Unit(gomock.Any(), names.NewUnitTag("foo/1")).Return(nil, &params.Error{Code: "not found"})

	logger := logger.GetLogger("test")
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("relations-tester", &tw), tc.IsNil)

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, logger)
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(context.Background(), 99, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		Level:   loggo.WARNING,
		Message: `unit foo/1 in relation 99 no longer exists`},
	})
}

func (s *stateManagerSuite) TestRemoveCachesUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	stateThree := relation.State{RelationId: 100}
	stateThree.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo, stateThree)
	s.expectSetState(c, stateThree)
	s.mockUnitGetter.EXPECT().Unit(gomock.Any(), names.NewUnitTag("foo/1")).Return(nil, &params.Error{Code: "not found"})

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	knownUnits := make(map[string]bool)
	err = mgr.RemoveRelation(context.Background(), 99, s.mockUnitGetter, knownUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(knownUnits, jc.DeepEquals, map[string]bool{"foo/1": false})

	s.expectSetStateEmpty(c)
	err = mgr.RemoveRelation(context.Background(), 100, s.mockUnitGetter, knownUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateManagerSuite) TestRemoveFailRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	s.expectState(c, stateTwo)
	s.expectSetStateError()

	mgr, err := relation.NewStateManager(context.Background(), s.mockUnitRW, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(context.Background(), 99, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
	found := mgr.RelationFound(99)
	c.Assert(found, jc.IsTrue)
}

var _ = tc.Suite(&stateManagerSuite{})

func (s *stateManagerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockUnitRW = mocks.NewMockUnitStateReadWriter(ctlr)
	s.mockUnitGetter = relmocks.NewMockUnitGetter(ctlr)
	return ctlr
}

func (s *stateManagerSuite) setupFourStates(c *tc.C) []relation.State {
	st1 := relation.NewState(123)
	st1.Members = map[string]int64{
		"foo/0": 1,
		"foo/1": 2,
	}
	st1.ChangedPending = "foo/1"
	st2 := relation.NewState(456)
	st2.Members = map[string]int64{
		"bar/0": 3,
		"bar/1": 4,
	}
	st3 := relation.NewState(789)
	st4 := relation.NewState(10)
	st4.ApplicationMembers = map[string]int64{
		"baz-app": 2,
	}
	states := []relation.State{*st1, *st2, *st3, *st4}
	s.expectState(c, states...)
	return states
}

func (s *stateManagerSuite) expectStateEmpty() {
	exp := s.mockUnitRW.EXPECT()
	exp.State(gomock.Any()).Return(params.UnitStateResult{}, nil)
}

func (s *stateManagerSuite) expectStateEmptyError() {
	exp := s.mockUnitRW.EXPECT()
	exp.State(gomock.Any()).Return(params.UnitStateResult{}, errors.BadRequestf("testing"))
}

func (s *stateManagerSuite) expectSetState(c *tc.C, states ...relation.State) {
	expectedStates := make(map[int]string, len(states))
	for _, s := range states {
		str, err := s.YamlString()
		c.Assert(err, jc.ErrorIsNil)
		expectedStates[s.RelationId] = str
	}
	exp := s.mockUnitRW.EXPECT()
	exp.SetState(gomock.Any(), unitStateMatcher{c: c, expected: expectedStates}).Return(nil)
}

func (s *stateManagerSuite) expectSetStateEmpty(c *tc.C) {
	exp := s.mockUnitRW.EXPECT()
	exp.SetState(gomock.Any(), unitStateMatcher{c: c, expected: map[int]string{}}).Return(nil)
}

func (s *stateManagerSuite) expectSetStateError() {
	exp := s.mockUnitRW.EXPECT()
	exp.SetState(gomock.Any(), gomock.Any()).Return(errors.BadRequestf("testing"))
}

func (s *stateManagerSuite) expectState(c *tc.C, states ...relation.State) {
	relationMap := make(map[int]string, len(states))
	for _, state := range states {
		data, err := yaml.Marshal(state)
		c.Assert(err, jc.ErrorIsNil)
		strState := string(data)
		relationMap[state.RelationId] = strState
	}
	exp := s.mockUnitRW.EXPECT()
	exp.State(gomock.Any()).Return(params.UnitStateResult{
		RelationState: relationMap,
	}, nil)
}

type unitStateMatcher struct {
	c        *tc.C
	expected map[int]string
}

func (m unitStateMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok {
		return false
	}

	m.c.Assert(*obtained.RelationState, tc.DeepEquals, m.expected)

	return true
}

func (m unitStateMatcher) String() string {
	return "Match the contents of the RelationState pointer in params.SetUnitStateArg"
}
