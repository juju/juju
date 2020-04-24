// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type SSHHostKeysSuite struct {
	ConnSuite
	machineTag names.MachineTag
}

var _ = gc.Suite(new(SSHHostKeysSuite))

func (s *SSHHostKeysSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.machineTag = s.Factory.MakeMachine(c, nil).MachineTag()
}

func (s *SSHHostKeysSuite) TestGetWithNoKeys(c *gc.C) {
	checkKeysNotFound(c, s.State, s.machineTag)
}

func (s *SSHHostKeysSuite) TestSetGet(c *gc.C) {
	for i := 0; i < 3; i++ {
		keys := state.SSHHostKeys{fmt.Sprintf("rsa foo %d", i), "dsa bar"}
		err := s.State.SetSSHHostKeys(s.machineTag, keys)
		c.Assert(err, jc.ErrorIsNil)
		checkGet(c, s.State, s.machineTag, keys)
	}
}

func (s *SSHHostKeysSuite) TestModelIsolation(c *gc.C) {
	stA := s.State
	tagA := s.machineTag
	keysA := state.SSHHostKeys{"rsaA", "dsaA"}
	c.Assert(stA.SetSSHHostKeys(tagA, keysA), jc.ErrorIsNil)

	stB := s.Factory.MakeModel(c, nil)
	defer stB.Close()
	factoryB := factory.NewFactory(stB, s.StatePool)
	tagB := factoryB.MakeMachine(c, nil).MachineTag()
	keysB := state.SSHHostKeys{"rsaB", "dsaB"}
	c.Assert(stB.SetSSHHostKeys(tagB, keysB), jc.ErrorIsNil)

	checkGet(c, stA, tagA, keysA)
	checkGet(c, stB, tagB, keysB)
}

func checkKeysNotFound(c *gc.C, st *state.State, tag names.MachineTag) {
	_, err := st.GetSSHHostKeys(tag)
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(err, gc.ErrorMatches, "keys not found")
}

func checkGet(c *gc.C, st *state.State, tag names.MachineTag, expected state.SSHHostKeys) {
	keysGot, err := st.GetSSHHostKeys(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(keysGot, gc.DeepEquals, expected)
}
