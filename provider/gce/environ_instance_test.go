// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
)

type environInstSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environInstSuite{})

func (s *environInstSuite) TestInstances(c *gc.C) {
	spam := s.NewInstance(c, "spam")
	ham := s.NewInstance(c, "ham")
	eggs := s.NewInstance(c, "eggs")
	s.FakeEnviron.Insts = []instance.Instance{spam, ham, eggs}

	ids := []instance.Id{"spam", "eggs", "ham"}
	insts, err := s.Env.Instances(s.CallCtx, ids)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(insts, jc.DeepEquals, []instance.Instance{spam, eggs, ham})
}

func (s *environInstSuite) TestInstancesEmptyArg(c *gc.C) {
	_, err := s.Env.Instances(s.CallCtx, nil)

	c.Check(err, gc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInstancesFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeEnviron.Err = failure

	ids := []instance.Id{"spam"}
	insts, err := s.Env.Instances(s.CallCtx, ids)

	c.Check(insts, jc.DeepEquals, []instance.Instance{nil})
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *environInstSuite) TestInstancesPartialMatch(c *gc.C) {
	s.FakeEnviron.Insts = []instance.Instance{s.Instance}

	ids := []instance.Id{"spam", "eggs"}
	insts, err := s.Env.Instances(s.CallCtx, ids)

	c.Check(insts, jc.DeepEquals, []instance.Instance{s.Instance, nil})
	c.Check(errors.Cause(err), gc.Equals, environs.ErrPartialInstances)
}

func (s *environInstSuite) TestInstancesNoMatch(c *gc.C) {
	s.FakeEnviron.Insts = []instance.Instance{s.Instance}

	ids := []instance.Id{"eggs"}
	insts, err := s.Env.Instances(s.CallCtx, ids)

	c.Check(insts, jc.DeepEquals, []instance.Instance{nil})
	c.Check(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestBasicInstances(c *gc.C) {
	spam := s.NewBaseInstance(c, "spam")
	ham := s.NewBaseInstance(c, "ham")
	eggs := s.NewBaseInstance(c, "eggs")
	s.FakeConn.Insts = []google.Instance{*spam, *ham, *eggs}

	insts, err := gce.GetInstances(s.Env)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(insts, jc.DeepEquals, []instance.Instance{
		s.NewInstance(c, "spam"),
		s.NewInstance(c, "ham"),
		s.NewInstance(c, "eggs"),
	})
}

func (s *environInstSuite) TestBasicInstancesAPI(c *gc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}

	_, err := gce.GetInstances(s.Env)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Instances")
	c.Check(s.FakeConn.Calls[0].Prefix, gc.Equals, s.Prefix())
	c.Check(s.FakeConn.Calls[0].Statuses, jc.DeepEquals, []string{google.StatusPending, google.StatusStaging, google.StatusRunning})
}

func (s *environInstSuite) TestControllerInstances(c *gc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}

	ids, err := s.Env.ControllerInstances(s.CallCtx, s.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, jc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestControllerInstancesAPI(c *gc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}

	_, err := s.Env.ControllerInstances(s.CallCtx, s.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Instances")
	c.Check(s.FakeConn.Calls[0].Prefix, gc.Equals, s.Prefix())
	c.Check(s.FakeConn.Calls[0].Statuses, jc.DeepEquals, []string{google.StatusPending, google.StatusStaging, google.StatusRunning})
}

func (s *environInstSuite) TestControllerInstancesNotBootstrapped(c *gc.C) {
	_, err := s.Env.ControllerInstances(s.CallCtx, s.ControllerUUID)

	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (s *environInstSuite) TestControllerInstancesMixed(c *gc.C) {
	other := google.NewInstance(google.InstanceSummary{}, nil)
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance, *other}

	ids, err := s.Env.ControllerInstances(s.CallCtx, s.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, jc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestParsePlacement(c *gc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}

	placement, err := gce.ParsePlacement(s.Env, "zone=a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(placement.Zone, jc.DeepEquals, &zone)
}

func (s *environInstSuite) TestParsePlacementZoneFailure(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	_, err := gce.ParsePlacement(s.Env, "zone=a-zone")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *environInstSuite) TestParsePlacementMissingDirective(c *gc.C) {
	_, err := gce.ParsePlacement(s.Env, "a-zone")

	c.Check(err, gc.ErrorMatches, `.*unknown placement directive: .*`)
}

func (s *environInstSuite) TestParsePlacementUnknownDirective(c *gc.C) {
	_, err := gce.ParsePlacement(s.Env, "inst=spam")

	c.Check(err, gc.ErrorMatches, `.*unknown placement directive: .*`)
}

func (s *environInstSuite) TestCheckInstanceType(c *gc.C) {
	typ := "n1-standard-1"
	cons := constraints.Value{
		InstanceType: &typ,
	}
	matched := gce.CheckInstanceType(cons)

	c.Check(matched, jc.IsTrue)
}

func (s *environInstSuite) TestCheckInstanceTypeUnknown(c *gc.C) {
	typ := "n1-standard-1.unknown"
	cons := constraints.Value{
		InstanceType: &typ,
	}
	matched := gce.CheckInstanceType(cons)

	c.Check(matched, jc.IsFalse)
}

func (s *environInstSuite) TestListMachineTypes(c *gc.C) {
	_, err := s.Env.InstanceTypes(s.CallCtx, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "no instance types in  matching constraints \"\"")

	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}

	mem := uint64(1025)
	types, err := s.Env.InstanceTypes(s.CallCtx, constraints.Value{Mem: &mem})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 1)

}

func (s *environInstSuite) TestAdoptResources(c *gc.C) {
	john := s.NewInstance(c, "john")
	misty := s.NewInstance(c, "misty")
	s.FakeEnviron.Insts = []instance.Instance{john, misty}

	err := s.Env.AdoptResources(s.CallCtx, "other-uuid", version.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.FakeConn.Calls, gc.HasLen, 1)
	call := s.FakeConn.Calls[0]
	c.Check(call.FuncName, gc.Equals, "UpdateMetadata")
	c.Check(call.IDs, gc.DeepEquals, []string{"john", "misty"})
	c.Check(call.Key, gc.Equals, tags.JujuController)
	c.Check(call.Value, gc.Equals, "other-uuid")
}
