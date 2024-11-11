// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	stdcontext "context"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	caasApplication "github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
)

type ConstraintsSuite struct {
	BaseSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *ConstraintsSuite) TestConstraintsValidatorOkay(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("mem=64G")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *ConstraintsSuite) TestConstraintsValidatorEmpty(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *ConstraintsSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(context.NewEmptyCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse(strings.Join([]string{
		"arch=amd64",
		"tags=foo",
		"mem=3",
		"instance-type=some-type",
		"cores=2",
		"cpu-power=250",
		"virt-type=kvm",
		"root-disk=10M",
		"spaces=foo",
		"container=kvm",
	}, " "))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		"cores",
		"virt-type",
		"instance-type",
		"spaces",
		"container",
	}
	c.Check(unsupported, jc.SameContents, expected)
}

func (s *ConstraintsSuite) TestConstraintsValidatorTopologyKeyTagConflictWithPodPrefix(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(context.WithoutCredentialInvalidator(stdcontext.Background()))
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("tags=" + caasApplication.PodPrefix + caasApplication.TopologyKeyTag + "=foo," + caasApplication.TopologySpreadPrefix + caasApplication.TopologyKeyTag + "=foo")

	val, err := validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "podAffinity/antiPodAffinity's topology-key and topologySpread's cannot have the same value")
	c.Assert(val, gc.IsNil)
}

func (s *ConstraintsSuite) TestConstraintsValidatorTopologyKeyTagConflictWithAntipodPrefix(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(context.WithoutCredentialInvalidator(stdcontext.Background()))
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("tags=" + caasApplication.AntiPodPrefix + caasApplication.TopologyKeyTag + "=foo," + caasApplication.TopologySpreadPrefix + caasApplication.TopologyKeyTag + "=foo")

	val, err := validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "podAffinity/antiPodAffinity's topology-key and topologySpread's cannot have the same value")
	c.Assert(val, gc.IsNil)
}

func (s *ConstraintsSuite) TestConstraintsValidatorTopologyKeyTagPass(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(context.WithoutCredentialInvalidator(stdcontext.Background()))
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("tags=" + caasApplication.PodPrefix + caasApplication.TopologyKeyTag + "=foo," + caasApplication.AntiPodPrefix + caasApplication.TopologyKeyTag + "=anti," + caasApplication.TopologySpreadPrefix + caasApplication.TopologyKeyTag + "=bar")

	val, err := validator.Validate(cons)
	c.Assert(err, gc.IsNil)
	// No unsupported constraints, hence this should be empty
	c.Assert(val, gc.IsNil)
}
