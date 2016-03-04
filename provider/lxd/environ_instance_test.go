// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/tools/lxdclient"
)

type environInstSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environInstSuite{})

func (s *environInstSuite) TestInstancesOkay(c *gc.C) {
	ids := []instance.Id{"spam", "eggs", "ham"}
	var raw []lxdclient.Instance
	var expected []instance.Instance
	for _, id := range ids {
		raw = append(raw, *s.NewRawInstance(c, string(id)))
		expected = append(expected, s.NewInstance(c, string(id)))
	}
	s.Client.Insts = raw

	insts, err := s.Env.Instances(ids)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(insts, jc.DeepEquals, expected)
}

func (s *environInstSuite) TestInstancesAPI(c *gc.C) {
	ids := []instance.Id{"spam", "eggs", "ham"}
	s.Env.Instances(ids)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "Instances",
		Args: []interface{}{
			s.Prefix + "machine-",
			lxdclient.AliveStatuses,
		},
	}})
}

func (s *environInstSuite) TestInstancesEmptyArg(c *gc.C) {
	insts, err := s.Env.Instances(nil)

	c.Check(insts, gc.HasLen, 0)
	c.Check(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInstancesFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.Stub.SetErrors(failure)

	ids := []instance.Id{"spam"}
	insts, err := s.Env.Instances(ids)

	c.Check(insts, jc.DeepEquals, []instance.Instance{nil})
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *environInstSuite) TestInstancesPartialMatch(c *gc.C) {
	raw := s.NewRawInstance(c, "spam")
	expected := s.NewInstance(c, "spam")
	s.Client.Insts = []lxdclient.Instance{*raw}

	ids := []instance.Id{"spam", "eggs"}
	insts, err := s.Env.Instances(ids)

	c.Check(insts, jc.DeepEquals, []instance.Instance{expected, nil})
	c.Check(errors.Cause(err), gc.Equals, environs.ErrPartialInstances)
}

func (s *environInstSuite) TestInstancesNoMatch(c *gc.C) {
	raw := s.NewRawInstance(c, "spam")
	s.Client.Insts = []lxdclient.Instance{*raw}

	ids := []instance.Id{"eggs"}
	insts, err := s.Env.Instances(ids)

	c.Check(insts, jc.DeepEquals, []instance.Instance{nil})
	c.Check(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestControllerInstancesOkay(c *gc.C) {
	s.Client.Insts = []lxdclient.Instance{*s.RawInstance}

	ids, err := s.Env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, jc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestControllerInstancesNotBootstrapped(c *gc.C) {
	_, err := s.Env.ControllerInstances()

	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (s *environInstSuite) TestControllerInstancesMixed(c *gc.C) {
	other := lxdclient.NewInstance(lxdclient.InstanceSummary{}, nil)
	s.Client.Insts = []lxdclient.Instance{*s.RawInstance, *other}

	ids, err := s.Env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, jc.DeepEquals, []instance.Id{"spam"})
}
