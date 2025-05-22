// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
)

type environInstSuite struct {
	gce.BaseSuite
}

func TestEnvironInstSuite(t *testing.T) {
	tc.Run(t, &environInstSuite{})
}

func (s *environInstSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	// NOTE(achilleasa): at least one zone is required so that any tests
	// that trigger a call to InstanceTypes can obtain a non-empty instance
	// list.
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}
}

func (s *environInstSuite) TestInstances(c *tc.C) {
	spam := s.NewInstance(c, "spam")
	ham := s.NewInstance(c, "ham")
	eggs := s.NewInstance(c, "eggs")
	s.FakeEnviron.Insts = []instances.Instance{spam, ham, eggs}

	ids := []instance.Id{"spam", "eggs", "ham"}
	insts, err := s.Env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(insts, tc.DeepEquals, []instances.Instance{spam, eggs, ham})
}

func (s *environInstSuite) TestInstancesEmptyArg(c *tc.C) {
	_, err := s.Env.Instances(c.Context(), nil)

	c.Check(err, tc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInstancesFailed(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeEnviron.Err = failure

	ids := []instance.Id{"spam"}
	insts, err := s.Env.Instances(c.Context(), ids)

	c.Check(insts, tc.DeepEquals, []instances.Instance{nil})
	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *environInstSuite) TestInstancesPartialMatch(c *tc.C) {
	s.FakeEnviron.Insts = []instances.Instance{s.Instance}

	ids := []instance.Id{"spam", "eggs"}
	insts, err := s.Env.Instances(c.Context(), ids)

	c.Check(insts, tc.DeepEquals, []instances.Instance{s.Instance, nil})
	c.Check(errors.Cause(err), tc.Equals, environs.ErrPartialInstances)
}

func (s *environInstSuite) TestInstancesNoMatch(c *tc.C) {
	s.FakeEnviron.Insts = []instances.Instance{s.Instance}

	ids := []instance.Id{"eggs"}
	insts, err := s.Env.Instances(c.Context(), ids)

	c.Check(insts, tc.DeepEquals, []instances.Instance{nil})
	c.Check(errors.Cause(err), tc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestBasicInstances(c *tc.C) {
	spam := s.NewBaseInstance(c, "spam")
	ham := s.NewBaseInstance(c, "ham")
	eggs := s.NewBaseInstance(c, "eggs")
	s.FakeConn.Insts = []google.Instance{*spam, *ham, *eggs}

	insts, err := gce.GetInstances(s.Env, c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(insts, tc.DeepEquals, []instances.Instance{
		s.NewInstance(c, "spam"),
		s.NewInstance(c, "ham"),
		s.NewInstance(c, "eggs"),
	})
}

func (s *environInstSuite) TestBasicInstancesAPI(c *tc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}

	_, err := gce.GetInstances(s.Env, c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "Instances")
	c.Check(s.FakeConn.Calls[0].Prefix, tc.Equals, s.Prefix())
	c.Check(s.FakeConn.Calls[0].Statuses, tc.DeepEquals, []string{google.StatusPending, google.StatusStaging, google.StatusRunning})
}

func (s *environInstSuite) TestControllerInstances(c *tc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}

	ids, err := s.Env.ControllerInstances(c.Context(), s.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestControllerInstancesAPI(c *tc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}

	_, err := s.Env.ControllerInstances(c.Context(), s.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "Instances")
	c.Check(s.FakeConn.Calls[0].Prefix, tc.Equals, s.Prefix())
	c.Check(s.FakeConn.Calls[0].Statuses, tc.DeepEquals, []string{google.StatusPending, google.StatusStaging, google.StatusRunning})
}

func (s *environInstSuite) TestControllerInstancesNotBootstrapped(c *tc.C) {
	_, err := s.Env.ControllerInstances(c.Context(), s.ControllerUUID)

	c.Check(err, tc.Equals, environs.ErrNotBootstrapped)
}

func (s *environInstSuite) TestControllerInstancesMixed(c *tc.C) {
	other := google.NewInstance(google.InstanceSummary{}, nil)
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance, *other}

	ids, err := s.Env.ControllerInstances(c.Context(), s.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestParsePlacement(c *tc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}

	placement, err := gce.ParsePlacement(s.Env, c.Context(), "zone=a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(placement.Zone, tc.DeepEquals, &zone)
}

func (s *environInstSuite) TestParsePlacementZoneFailure(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	_, err := gce.ParsePlacement(s.Env, c.Context(), "zone=a-zone")

	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *environInstSuite) TestParsePlacementMissingDirective(c *tc.C) {
	_, err := gce.ParsePlacement(s.Env, c.Context(), "a-zone")

	c.Check(err, tc.ErrorMatches, `.*unknown placement directive: .*`)
}

func (s *environInstSuite) TestParsePlacementUnknownDirective(c *tc.C) {
	_, err := gce.ParsePlacement(s.Env, c.Context(), "inst=spam")

	c.Check(err, tc.ErrorMatches, `.*unknown placement directive: .*`)
}

func (s *environInstSuite) TestPrecheckInstanceWithValidInstanceType(c *tc.C) {
	typ := "n1-standard-2"
	err := s.Env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Constraints: constraints.Value{
			InstanceType: &typ,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environInstSuite) TestPrecheckInstanceTypeUnknown(c *tc.C) {
	typ := "bogus"
	err := s.Env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Constraints: constraints.Value{
			InstanceType: &typ,
		},
	})
	c.Assert(err, tc.ErrorMatches, `.*invalid GCE instance type "bogus".*`)
}

func (s *environInstSuite) TestPrecheckInstanceInvalidCredentialError(c *tc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}
	mem := uint64(1025)
	s.FakeConn.Err = gce.InvalidCredentialError

	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.Env.InstanceTypes(c.Context(), constraints.Value{Mem: &mem})
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environInstSuite) TestListMachineTypes(c *tc.C) {
	// If no zone is specified, no machine types will be pulled.
	s.FakeConn.Zones = nil
	_, err := s.Env.InstanceTypes(c.Context(), constraints.Value{})
	c.Assert(err, tc.ErrorMatches, "no instance types in  matching constraints.*")

	// If a non-empty list of zones is specified , we will make an API call
	// to fetch the available machine types.
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}

	mem := uint64(1025)
	types, err := s.Env.InstanceTypes(c.Context(), constraints.Value{Mem: &mem})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types.InstanceTypes, tc.HasLen, 1)

}

func (s *environInstSuite) TestAdoptResources(c *tc.C) {
	john := s.NewInstance(c, "john")
	misty := s.NewInstance(c, "misty")
	s.FakeEnviron.Insts = []instances.Instance{john, misty}

	err := s.Env.AdoptResources(c.Context(), "other-uuid", semversion.MustParse("1.2.3"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.FakeConn.Calls, tc.HasLen, 1)
	call := s.FakeConn.Calls[0]
	c.Check(call.FuncName, tc.Equals, "UpdateMetadata")
	c.Check(call.IDs, tc.DeepEquals, []string{"john", "misty"})
	c.Check(call.Key, tc.Equals, tags.JujuController)
	c.Check(call.Value, tc.Equals, "other-uuid")
}

func (s *environInstSuite) TestAdoptResourcesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	john := s.NewInstance(c, "john")
	misty := s.NewInstance(c, "misty")
	s.FakeEnviron.Insts = []instances.Instance{john, misty}

	err := s.Env.AdoptResources(c.Context(), "other-uuid", semversion.MustParse("1.2.3"))
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}
