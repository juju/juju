// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state"
)

type FakeSpaceState struct {
}

func (st *FakeSpaceState) EnvironUUID() string {
	return ""
}

func (st *FakeSpaceState) Space(name string) (*state.Space, error) {
	return nil, nil
}

var _ = gc.Suite(&SpaceSuite{})

type TestMeta struct {
	Ops      [][]txn.Op
	ErrCalls int
	Errors   []error
}

func (m *TestMeta) nextError() (err error) {
	if len(m.Errors) < m.ErrCalls {
		err = m.Errors[m.ErrCalls]
		m.ErrCalls++
	}
	return err
}

func (m *TestMeta) runTransaction(st state.SpaceState, ops []txn.Op) error {
	m.Ops = append(m.Ops, ops)
	return m.nextError()
}

type SpaceSuite struct {
	m TestMeta
}

func (s *SpaceSuite) SetUpTest(c *gc.C) {
	s.m = TestMeta{}
}

func (s *SpaceSuite) TestAddSpace(c *gc.C) {
	st := &FakeSpaceState{}
	state.AddSpace(st, "MySpace", []string{"1.1.1.0/24"}, false, "MySpaceID", s.m.runTransaction)

	spaceDoc := state.SpaceDoc{
		DocID:    "MySpaceID",
		EnvUUID:  st.EnvironUUID(),
		Life:     state.Alive,
		Name:     "MySpace",
		Subnets:  []string{"1.1.1.0/24"},
		IsPublic: false,
	}
	expectedOps := []txn.Op{{
		C:      "spaces",
		Id:     "MySpaceID",
		Assert: txn.DocMissing,
		Insert: spaceDoc,
	}}
	c.Assert(len(s.m.Ops), gc.Equals, 1)
	c.Assert(s.m.Ops[0], jc.DeepEquals, expectedOps)
}
