// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/lxd"
)

type environPolicySuite struct {
	lxd.EnvironSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&environPolicySuite{})

func (s *environPolicySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewCloudCallContext()
}

func (s *environPolicySuite) TestPrecheckInstanceDefaults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)
	err := env.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{Series: series.DefaultSupportedLTS()})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceHasInstanceType(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	cons := constraints.MustParse("instance-type=some-instance-type")
	err := env.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{Series: series.DefaultSupportedLTS(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceDiskSize(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	cons := constraints.MustParse("root-disk=1G")
	err := env.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{Series: series.DefaultSupportedLTS(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	cons := constraints.MustParse("arch=i386")

	env := s.NewEnviron(c, svr, nil)
	err := env.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{Series: series.DefaultSupportedLTS(), Constraints: cons})

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

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

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.IsClustered().Return(true),
		exp.GetClusterMembers().Return(members, nil),
	)

	placement := "zone=a-zone"
	err := env.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{Series: series.DefaultSupportedLTS(), Placement: placement})

	c.Check(err, gc.ErrorMatches, `availability zone "a-zone" not valid`)
}

func (s *environPolicySuite) TestConstraintsValidatorOkay(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolicySuite) TestConstraintsValidatorEmpty(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolicySuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse(strings.Join([]string{
		"arch=amd64",
		"tags=foo",
		"mem=3",
		"instance-type=some-type",
		"cores=2",
		"cpu-power=250",
		"virt-type=kvm",
	}, " "))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		"tags",
		"cpu-power",
		"virt-type",
	}
	c.Check(unsupported, jc.SameContents, expected)
}

func (s *environPolicySuite) TestConstraintsValidatorVocabArchKnown(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	_, err = validator.Validate(cons)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolicySuite) TestConstraintsValidatorVocabArchUnknown(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: \\[amd64\\]")
}

func (s *environPolicySuite) TestConstraintsValidatorVocabContainerUnknown(c *gc.C) {
	c.Skip("this will fail until we add a container vocabulary")
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("container=lxd")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: container=lxd\nvalid values are:.*")
}

func (s *environPolicySuite) TestConstraintsValidatorConflicts(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	exp := svr.EXPECT()
	exp.HostArch().Return(arch.AMD64)

	validator, err := env.ConstraintsValidator(context.NewCloudCallContext())
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)

	isSupported := env.(interface {
		SupportNetworks(context.ProviderCallContext) bool
	}).SupportNetworks(context.NewCloudCallContext())

	c.Check(isSupported, jc.IsFalse)
}
