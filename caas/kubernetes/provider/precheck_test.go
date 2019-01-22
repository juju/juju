// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

type PrecheckSuite struct {
	BaseSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&PrecheckSuite{})

func (s *PrecheckSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewCloudCallContext()
}

func (s *PrecheckSuite) TestSuccess(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{
		Series:      "kubernetes",
		Constraints: constraints.MustParse("mem=4G"),
		Placement:   "a=b",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PrecheckSuite) TestWrongSeries(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{
		Series: "quantal",
	})
	c.Assert(err, gc.ErrorMatches, `series "quantal" not valid`)
}

func (s *PrecheckSuite) TestUnsupportedConstraints(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{
		Series:      "kubernetes",
		Constraints: constraints.MustParse("instance-type=foo"),
	})
	c.Assert(err, gc.ErrorMatches, `constraints instance-type not supported`)
}

func (s *PrecheckSuite) TestBadPlacement(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(context.NewCloudCallContext(), environs.PrecheckInstanceParams{
		Series:    "kubernetes",
		Placement: "a",
	})
	c.Assert(err, gc.ErrorMatches, `placement directive "a" not valid`)
}
