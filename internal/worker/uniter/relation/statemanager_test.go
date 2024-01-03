// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/relation"
	relmocks "github.com/juju/juju/internal/worker/uniter/relation/mocks"
	"github.com/juju/juju/rpc/params"
)

type stateManagerSuite struct {
	mockUnitRW     *mocks.MockUnitStateReadWriter
	mockUnitGetter *relmocks.MockUnitGetter
}

func (s *stateManagerSuite) TestNewStateManagerHasState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	for _, st := range states {
		v, err := mgr.Relation(st.RelationId)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*v, gc.DeepEquals, st)
	}
}

func (s *stateManagerSuite) TestNewStateManagerNoState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateEmpty()

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mgr.KnownIDs(), gc.HasLen, 0)
}

func (s *stateManagerSuite) TestNewStateManagerErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateEmptyError()

	_, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *stateManagerSuite) TestKnownIds(c *gc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	ids := mgr.KnownIDs()
	intSet := set.NewInts(ids...)
	c.Assert(intSet.Size(), gc.Equals, 4, gc.Commentf("obtained %v", intSet.Values()))
	for _, exp := range states {
		c.Assert(intSet.Contains(exp.RelationId), jc.IsTrue)
	}
}

func (s *stateManagerSuite) TestRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	st, err := mgr.Relation(states[1].RelationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*st, gc.DeepEquals, states[1])
}

func (s *stateManagerSuite) TestRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_ = s.setupFourStates(c)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = mgr.Relation(42)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateManagerSuite) TestSetNew(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateEmpty()
	st2 := &relation.State{RelationId: 456}
	st2.Members = map[string]int64{
		"bar/0": 3,
		"bar/1": 4,
	}
	s.expectSetState(c, *st2)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.SetRelation(st2)
	c.Assert(err, jc.ErrorIsNil)
	found := mgr.RelationFound(456)
	c.Assert(found, jc.IsTrue)
}

func (s *stateManagerSuite) TestSetChangeExisting(c *gc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	states[3].ChangedPending = "foo/1"
	s.expectSetState(c, states...)
	st := states[3]

	err = mgr.SetRelation(&st)
	c.Assert(err, jc.ErrorIsNil)

	obtained, err := mgr.Relation(st.RelationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtained, gc.DeepEquals, st)
}

func (s *stateManagerSuite) TestSetChangeExistingFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	states := s.setupFourStates(c)
	s.expectSetStateError()

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	st := states[3]
	st.ChangedPending = "foo/1"
	err = mgr.SetRelation(&st)
	c.Assert(err, jc.ErrorIs, errors.BadRequest)

	obtained, err := mgr.Relation(st.RelationId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtained, gc.DeepEquals, states[3])
}

func (s *stateManagerSuite) TestRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()
	state := relation.State{RelationId: 1}
	s.expectState(c, state)
	s.expectSetStateEmpty(c)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(1, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateManagerSuite) TestRemoveNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(1, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateManagerSuite) TestRemoveFailHasMembers(c *gc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo)
	s.mockUnitGetter.EXPECT().Unit(names.NewUnitTag("foo/1")).Return(nil, nil)

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(99, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, gc.ErrorMatches, `*has members: \[foo/1\]`)
}

func (s *stateManagerSuite) TestRemoveIgnoresMissingUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo)
	s.expectSetStateEmpty(c)
	s.mockUnitGetter.EXPECT().Unit(names.NewUnitTag("foo/1")).Return(nil, &params.Error{Code: "not found"})

	logger := loggo.GetLogger("test")
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("relations-tester", &tw), gc.IsNil)

	mgr, err := relation.NewStateManager(s.mockUnitRW, logger)
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(99, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`unit foo/1 in relation 99 no longer exists`},
	})
}

func (s *stateManagerSuite) TestRemoveCachesUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	stateTwo.Members = map[string]int64{"foo/1": 0}
	stateThree := relation.State{RelationId: 100}
	stateThree.Members = map[string]int64{"foo/1": 0}
	s.expectState(c, stateTwo, stateThree)
	s.expectSetState(c, stateThree)
	s.mockUnitGetter.EXPECT().Unit(names.NewUnitTag("foo/1")).Return(nil, &params.Error{Code: "not found"})

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	knownUnits := make(map[string]bool)
	err = mgr.RemoveRelation(99, s.mockUnitGetter, knownUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(knownUnits, jc.DeepEquals, map[string]bool{"foo/1": false})

	s.expectSetStateEmpty(c)
	err = mgr.RemoveRelation(100, s.mockUnitGetter, knownUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateManagerSuite) TestRemoveFailRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()
	stateTwo := relation.State{RelationId: 99}
	s.expectState(c, stateTwo)
	s.expectSetStateError()

	mgr, err := relation.NewStateManager(s.mockUnitRW, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	err = mgr.RemoveRelation(99, s.mockUnitGetter, map[string]bool{})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
	found := mgr.RelationFound(99)
	c.Assert(found, jc.IsTrue)
}

var _ = gc.Suite(&stateManagerSuite{})

func (s *stateManagerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockUnitRW = mocks.NewMockUnitStateReadWriter(ctlr)
	s.mockUnitGetter = relmocks.NewMockUnitGetter(ctlr)
	return ctlr
}

func (s *stateManagerSuite) setupFourStates(c *gc.C) []relation.State {
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
	exp.State().Return(params.UnitStateResult{}, nil)
}

func (s *stateManagerSuite) expectStateEmptyError() {
	exp := s.mockUnitRW.EXPECT()
	exp.State().Return(params.UnitStateResult{}, errors.BadRequestf("testing"))
}

func (s *stateManagerSuite) expectSetState(c *gc.C, states ...relation.State) {
	expectedStates := make(map[int]string, len(states))
	for _, s := range states {
		str, err := s.YamlString()
		c.Assert(err, jc.ErrorIsNil)
		expectedStates[s.RelationId] = str
	}
	exp := s.mockUnitRW.EXPECT()
	exp.SetState(unitStateMatcher{c: c, expected: expectedStates}).Return(nil)
}

func (s *stateManagerSuite) expectSetStateEmpty(c *gc.C) {
	exp := s.mockUnitRW.EXPECT()
	exp.SetState(unitStateMatcher{c: c, expected: map[int]string{}}).Return(nil)
}

func (s *stateManagerSuite) expectSetStateError() {
	exp := s.mockUnitRW.EXPECT()
	exp.SetState(gomock.Any()).Return(errors.BadRequestf("testing"))
}

func (s *stateManagerSuite) expectState(c *gc.C, states ...relation.State) {
	relationMap := make(map[int]string, len(states))
	for _, state := range states {
		data, err := yaml.Marshal(state)
		c.Assert(err, jc.ErrorIsNil)
		strState := string(data)
		relationMap[state.RelationId] = strState
	}
	exp := s.mockUnitRW.EXPECT()
	exp.State().Return(params.UnitStateResult{
		RelationState: relationMap,
	}, nil)
}

type unitStateMatcher struct {
	c        *gc.C
	expected map[int]string
}

func (m unitStateMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok {
		return false
	}

	m.c.Assert(*obtained.RelationState, gc.DeepEquals, m.expected)

	return true
}

func (m unitStateMatcher) String() string {
	return "Match the contents of the RelationState pointer in params.SetUnitStateArg"
}
