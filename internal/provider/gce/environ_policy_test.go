// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/storage"
)

type environPolSuite struct {
	gce.BaseSuite
}

func TestEnvironPolSuite(t *testing.T) {
	tc.Run(t, &environPolSuite{})
}

func (s *environPolSuite) TestPrecheckInstanceDefaults(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase()})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceFull(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil).Times(2)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           ptr(uint64(0)),
		Name:         ptr("n1-standard-2"),
		GuestCpus:    ptr(int32(2)),
		Architecture: ptr("amd64"),
	}}, nil)

	cons := constraints.MustParse("instance-type=n1-standard-2 arch=amd64 root-disk=1G")
	placement := "zone=home-zone"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons, Placement: placement})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceValidInstanceType(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           ptr(uint64(0)),
		Name:         ptr("n1-standard-2"),
		GuestCpus:    ptr(int32(2)),
		Architecture: ptr("amd64"),
	}}, nil)

	cons := constraints.MustParse("instance-type=n1-standard-2")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceInvalidInstanceType(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           ptr(uint64(0)),
		Name:         ptr("n1-standard-1"),
		GuestCpus:    ptr(int32(2)),
		Architecture: ptr("amd64"),
	}}, nil)

	cons := constraints.MustParse("instance-type=n1-standard-1.invalid")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Assert(err, tc.ErrorMatches, `.*invalid GCE instance type.*`)
}

func (s *environPolSuite) TestPrecheckInstanceUnsupportedArch(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           ptr(uint64(0)),
		Name:         ptr("n1-standard-2"),
		GuestCpus:    ptr(int32(2)),
		Architecture: ptr("amd64"),
	}}, nil)

	cons := constraints.MustParse("instance-type=n1-standard-2 arch=arm64")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZone(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("a-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("b-zone"),
		Status: ptr("UP"),
	}}, nil)

	placement := "zone=a-zone"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneUnavailable(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("a-zone"),
		Status: ptr("DOWN"),
	}}, nil)

	placement := "zone=a-zone"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Assert(err, tc.ErrorMatches, `.*availability zone "a-zone" is DOWN`)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneUnknown(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)

	placement := "zone=a-zone"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *environPolSuite) TestPrecheckInstanceVolumeAvailZoneNoPlacement(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      version.DefaultSupportedLTSBase(),
		Placement: "",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceVolumeAvailZoneSameZonePlacement(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("away-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)

	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      version.DefaultSupportedLTSBase(),
		Placement: "zone=away-zone",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			VolumeId: "away-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneConflictsVolume(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}, {
		Name:   ptr("away-zone"),
		Status: ptr("UP"),
	}}, nil)

	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      version.DefaultSupportedLTSBase(),
		Placement: "zone=away-zone",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			VolumeId: "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		}},
	})

	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=away-zone", as this will prevent attaching the requested disks in zone "home-zone"`)
}

func (s *environPolSuite) expectConstraintsCalls() {
	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   ptr("home-zone"),
		Status: ptr("UP"),
	}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           ptr(uint64(0)),
		Name:         ptr("n1-standard-2"),
		GuestCpus:    ptr(int32(2)),
		Architecture: ptr("amd64"),
	}}, nil)
}

func (s *environPolSuite) TestConstraintsValidator(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectConstraintsCalls()

	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unsupported, tc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorEmpty(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectConstraintsCalls()

	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(unsupported, tc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorUnsupported(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectConstraintsCalls()

	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(unsupported, tc.SameContents, []string{"tags", "virt-type"})
}

func (s *environPolSuite) TestConstraintsValidatorVocabInstType(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectConstraintsCalls()

	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)

	c.Assert(err, tc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorVocabContainer(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectConstraintsCalls()

	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("container=lxd")
	_, err = validator.Validate(cons)

	c.Assert(err, tc.ErrorMatches, "invalid constraint value: container=lxd\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorConflicts(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.expectConstraintsCalls()

	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=n1-standard-2")
	// We do not check arch or container since there is only one valid
	// value for each and will always match.
	consFallback := constraints.MustParse("cores=2 cpu-power=1000 mem=10000 tags=bar")
	merged, err := validator.Merge(consFallback, cons)
	c.Assert(err, tc.ErrorIsNil)

	// tags is not supported, but we're not validating here...
	expected := constraints.MustParse("instance-type=n1-standard-2 tags=bar")
	c.Assert(merged, tc.DeepEquals, expected)
}

func (s *environPolSuite) TestSupportNetworks(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	isSupported := env.SupportNetworks(c.Context())

	c.Assert(isSupported, tc.IsFalse)
}
