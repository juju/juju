// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type ControllerNodeSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerNodeSuite{})

func (s *ControllerNodeSuite) TestAddControllerNode(c *gc.C) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.IsManager(), jc.IsTrue)
	c.Assert(node.Tag().String(), gc.Equals, "controller-0")
	c.Assert(node.Life(), gc.Equals, state.Alive)
	node0, err := s.State.ControllerNode("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node, jc.DeepEquals, node0)

	// Check id increments.
	node1, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node1.Id(), gc.Equals, "1")
}

func (s *ControllerNodeSuite) TestSetPassword(c *gc.C) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	testSetPassword(c, s.State, func() (state.Authenticator, error) {
		return node, nil
	})
}

func (s *ControllerNodeSuite) TestSetMongoPassword(c *gc.C) {
	_, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	testSetMongoPassword(c, func(st *state.State, id string) (mongoPasswordSetter, error) {
		return st.ControllerNode("0")
	}, s.State.ControllerTag(), s.modelTag, s.Session)
}

func (s *ControllerNodeSuite) TestAgentTools(c *gc.C) {
	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	testAgentTools(c, state.NewObjectStore(c, s.State), node, "controller "+node.Id())
}
