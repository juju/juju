// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/storage"
)

type environPolSuite struct {
	gce.BaseSuite
}

var _ = tc.Suite(&environPolSuite{})

func (s *environPolSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	// NOTE(achilleasa): at least one zone is required so that any tests
	// that trigger a call to InstanceTypes can obtain a non-empty instance
	// list.
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}
}

func (s *environPolSuite) TestPrecheckInstanceDefaults(c *tc.C) {
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase()})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 0)
}

func (s *environPolSuite) TestPrecheckInstanceFullAPI(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp, "", ""),
	}

	cons := constraints.MustParse("instance-type=n1-standard-2 arch=amd64 root-disk=1G")
	placement := "zone=home-zone"
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons, Placement: placement})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, tc.Equals, "us-east1")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[1].Region, tc.Equals, "us-east1")
	// NOTE(achilleas): If the constraint specifies an instance type,
	// the precheck logic will fetch the machine types for the current zone
	// to validate the constraint value.
	c.Check(s.FakeConn.Calls[2].FuncName, tc.Equals, "ListMachineTypes")
	c.Check(s.FakeConn.Calls[2].ZoneName, tc.Equals, "home-zone")
}

func (s *environPolSuite) TestPrecheckInstanceValidInstanceType(c *tc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-2")
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceInvalidInstanceType(c *tc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-1.invalid")
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Check(err, tc.ErrorMatches, `.*invalid GCE instance type.*`)
}

func (s *environPolSuite) TestPrecheckInstanceDiskSize(c *tc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-2 root-disk=1G")
	placement := ""
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons, Placement: placement})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceUnsupportedArch(c *tc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-2 arch=arm64")
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZone(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
	}

	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneUnavailable(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusDown, "", ""),
	}

	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Check(err, tc.ErrorMatches, `.*availability zone "a-zone" is DOWN`)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneUnknown(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp, "", ""),
	}

	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (s *environPolSuite) TestPrecheckInstanceVolumeAvailZoneNoPlacement(c *tc.C) {
	s.testPrecheckInstanceVolumeAvailZone(c, "")
}

func (s *environPolSuite) TestPrecheckInstanceVolumeAvailZoneSameZonePlacement(c *tc.C) {
	s.testPrecheckInstanceVolumeAvailZone(c, "zone=away-zone")
}

func (s *environPolSuite) testPrecheckInstanceVolumeAvailZone(c *tc.C, placement string) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("away-zone", google.StatusUp, "", ""),
	}

	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base:      version.DefaultSupportedLTSBase(),
		Placement: placement,
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		}},
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneConflictsVolume(c *tc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("away-zone", google.StatusUp, "", ""),
	}

	err := s.Env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Base:      version.DefaultSupportedLTSBase(),
		Placement: "zone=away-zone",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		}},
	})

	c.Check(err, tc.ErrorMatches, `cannot create instance with placement "zone=away-zone", as this will prevent attaching the requested disks in zone "home-zone"`)
}

func (s *environPolSuite) TestConstraintsValidator(c *tc.C) {
	validator, err := s.Env.ConstraintsValidator(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unsupported, tc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorEmpty(c *tc.C) {
	validator, err := s.Env.ConstraintsValidator(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorUnsupported(c *tc.C) {
	validator, err := s.Env.ConstraintsValidator(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}

func (s *environPolSuite) TestConstraintsValidatorVocabInstType(c *tc.C) {
	validator, err := s.Env.ConstraintsValidator(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)

	c.Check(err, tc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorVocabContainer(c *tc.C) {
	validator, err := s.Env.ConstraintsValidator(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("container=lxd")
	_, err = validator.Validate(cons)

	c.Check(err, tc.ErrorMatches, "invalid constraint value: container=lxd\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorConflicts(c *tc.C) {
	validator, err := s.Env.ConstraintsValidator(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=n1-standard-2")
	// We do not check arch or container since there is only one valid
	// value for each and will always match.
	consFallback := constraints.MustParse("cores=2 cpu-power=1000 mem=10000 tags=bar")
	merged, err := validator.Merge(consFallback, cons)
	c.Assert(err, jc.ErrorIsNil)

	// tags is not supported, but we're not validating here...
	expected := constraints.MustParse("instance-type=n1-standard-2 tags=bar")
	c.Check(merged, jc.DeepEquals, expected)
}

func (s *environPolSuite) TestSupportNetworks(c *tc.C) {
	isSupported := s.Env.SupportNetworks(context.Background())

	c.Check(isSupported, jc.IsFalse)
}
