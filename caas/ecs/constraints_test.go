// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
)

type ConstraintsSuite struct {
	baseSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.callCtx = context.NewCloudCallContext()
}

func (s *ConstraintsSuite) TestConstraintsValidatorOkay(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.environ.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("mem=64G")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *ConstraintsSuite) TestConstraintsValidatorEmpty(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.environ.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *ConstraintsSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.environ.ConstraintsValidator(context.NewCloudCallContext())
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
		"arch",
		"instance-type",
		"spaces",
		"container",
	}
	c.Check(unsupported, jc.SameContents, expected)
}
