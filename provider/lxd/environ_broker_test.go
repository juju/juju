// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/lxd"
)

type environBrokerSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	s.Client.Inst = s.RawInstance

	result, err := s.Env.StartInstance(s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, gc.DeepEquals, s.Instance)
	c.Check(result.Hardware, gc.DeepEquals, s.HWC)
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	err := s.Env.StopInstances(s.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "RemoveInstances",
		Args: []interface{}{
			"juju-2d02eeac-9dbb-11e4-89d3-123b93f75cba-machine-",
			[]string{"spam"},
		},
	}})
}
