// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"fmt"
	"strings"

	"github.com/canonical/lxd/shared/api"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/version"
)

type environPolicySuite struct {
	lxd.EnvironSuite

	svr     *lxd.MockServer
	env     environs.Environ
	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&environPolicySuite{})

func (s *environPolicySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *environPolicySuite) TestPrecheckInstanceDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()
	err := s.env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Base: version.DefaultSupportedLTSBase()})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceHasInstanceType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cons := constraints.MustParse("instance-type=some-instance-type")
	err := s.env.PrecheckInstance(
		s.callCtx, environs.PrecheckInstanceParams{Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceDiskSize(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cons := constraints.MustParse("root-disk=1G")
	err := s.env.PrecheckInstance(
		s.callCtx, environs.PrecheckInstanceParams{Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cons := constraints.MustParse("arch=arm64")
	err := s.env.PrecheckInstance(
		s.callCtx, environs.PrecheckInstanceParams{Base: version.DefaultSupportedLTSBase(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	defer s.setupMocks(c).Finish()

	members := []api.ClusterMember{
		{
			ServerName: "node01",
			Status:     "ONLINE",
		},
		{
			ServerName: "node02",
			Status:     "ONLINE",
		},
	}

	exp := s.svr.EXPECT()
	gomock.InOrder(
		exp.IsClustered().Return(true),
		exp.GetClusterMembers().Return(members, nil),
	)

	placement := "zone=a-zone"
	err := s.env.PrecheckInstance(
		s.callCtx, environs.PrecheckInstanceParams{Base: version.DefaultSupportedLTSBase(), Placement: placement})

	c.Check(err, gc.ErrorMatches, `availability zone "a-zone" not valid`)
}

func (s *environPolicySuite) TestConstraintsValidatorArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolicySuite) TestConstraintsValidatorArchWithUnsupportedArches(c *gc.C) {
	// Don't use setupMocks because we need to mock SupportedArches
	// to return a list of unsupported arches.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.svr = lxd.NewMockServer(ctrl)
	s.svr.EXPECT().SupportedArches().Return([]string{arch.AMD64, arch.ARM64, "i386", "armhf", "ppc64"}).MaxTimes(1)

	s.env = s.NewEnviron(c, s.svr, nil, environscloudspec.CloudSpec{})

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	for _, arches := range []string{"arm64", "amd64"} {
		cons := constraints.MustParse(fmt.Sprintf("arch=%s", arches))
		unsupported, err := validator.Validate(cons)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(unsupported, gc.HasLen, 0)
	}
}

func (s *environPolicySuite) TestConstraintsValidatorVirtType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("virt-type=container")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolicySuite) TestConstraintsValidatorEmptyVirtType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("virt-type=")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolicySuite) TestConstraintsValidatorEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolicySuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse(strings.Join([]string{
		"arch=amd64",
		"tags=foo",
		"mem=3",
		"instance-type=some-type",
		"cores=2",
		"cpu-power=250",
		"virt-type=virtual-machine",
	}, " "))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		"tags",
		"cpu-power",
	}
	c.Check(unsupported, jc.SameContents, expected)
}

func (s *environPolicySuite) TestConstraintsValidatorVocabArchKnown(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	_, err = validator.Validate(cons)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestConstraintsValidatorVocabArchUnknown(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: amd64")
}

func (s *environPolicySuite) TestConstraintsValidatorVocabContainerUnknown(c *gc.C) {
	c.Skip("this will fail until we add a container vocabulary")

	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("container=lxd")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: container=lxd\nvalid values are:.*")
}

func (s *environPolicySuite) TestConstraintsValidatorConflicts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=n1-standard-1")
	consFallback := constraints.MustParse("cores=2 cpu-power=1000 mem=10000 tags=bar")
	merged, err := validator.Merge(consFallback, cons)
	c.Assert(err, jc.ErrorIsNil)

	// tags is not supported, but we're not validating here...
	expected := constraints.MustParse("instance-type=n1-standard-1 tags=bar cores=2 cpu-power=1000 mem=10000")
	c.Check(merged, jc.DeepEquals, expected)
}

func (s *environPolicySuite) TestSupportNetworks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isSupported := s.env.(interface {
		SupportNetworks(context.ProviderCallContext) bool
	}).SupportNetworks(context.NewEmptyCloudCallContext())

	c.Check(isSupported, jc.IsFalse)
}

func (s *environPolicySuite) TestShouldApplyControllerConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cons := constraints.MustParse("")

	ok := s.env.(environs.DefaultConstraintsChecker).ShouldApplyControllerConstraints(cons)
	c.Assert(ok, jc.IsFalse)
}

func (s *environPolicySuite) TestShouldApplyControllerConstraintsInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cons := constraints.MustParse("virt-type=invalid")

	ok := s.env.(environs.DefaultConstraintsChecker).ShouldApplyControllerConstraints(cons)
	c.Assert(ok, jc.IsFalse)
}

func (s *environPolicySuite) TestShouldApplyControllerConstraintsForVirtualMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cons := constraints.MustParse("virt-type=virtual-machine")

	ok := s.env.(environs.DefaultConstraintsChecker).ShouldApplyControllerConstraints(cons)
	c.Assert(ok, jc.IsTrue)
}

func (s *environPolicySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.svr = lxd.NewMockServer(ctrl)
	s.svr.EXPECT().SupportedArches().Return([]string{arch.AMD64}).MaxTimes(1)

	s.env = s.NewEnviron(c, s.svr, nil, environscloudspec.CloudSpec{})

	return ctrl
}
