// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
)

type ConstraintsSuite struct {
	BaseSuite
}

func TestConstraintsSuite(t *testing.T) {
	tc.Run(t, &ConstraintsSuite{})
}

func (s *ConstraintsSuite) TestConstraintsValidatorOkay(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("mem=64G")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)
}

func (s *ConstraintsSuite) TestConstraintsValidatorEmpty(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)
}

func (s *ConstraintsSuite) TestConstraintsValidatorUnsupported(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	validator, err := s.broker.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse(strings.Join([]string{
		"arch=amd64",
		"tags=foo",
		"mem=3",
		"instance-type=some-type",
		"cores=2",
		"cpu-power=250",
		"virt-type=lxd",
		"root-disk=10M",
		"spaces=foo",
		"container=lxd",
	}, " "))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	expected := []string{
		"cores",
		"virt-type",
		"instance-type",
		"spaces",
		"container",
	}
	c.Check(unsupported, tc.SameContents, expected)
}
