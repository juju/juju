// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
)

type PrecheckSuite struct {
	BaseSuite
}

var _ = gc.Suite(&PrecheckSuite{})

func (s *PrecheckSuite) TestSuccess(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("mem=4G"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PrecheckSuite) TestWrongSeries(c *gc.C) {
	c.Skip("disable for now because TODO(new-charms): handle systems")

	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.PrecheckInstanceParams{
		Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
	})
	c.Assert(err, gc.ErrorMatches, `series "quantal" not valid`)
}

func (s *PrecheckSuite) TestUnsupportedConstraints(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("instance-type=foo"),
	})
	c.Assert(err, gc.ErrorMatches, `constraints instance-type not supported`)
}

func (s *PrecheckSuite) TestPlacementNotAllowed(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.PrecheckInstanceParams{
		Base:      corebase.MakeDefaultBase("ubuntu", "22.04"),
		Placement: "a",
	})
	c.Assert(err, gc.ErrorMatches, `placement directive "a" not valid`)
}

func (s *PrecheckSuite) TestInvalidConstraints(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.PrecheckInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("tags=foo"),
	})
	c.Assert(err, gc.ErrorMatches, `invalid node affinity constraints: foo`)
	err = s.broker.PrecheckInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "22.04"),
		Constraints: constraints.MustParse("tags=^=bar"),
	})
	c.Assert(err, gc.ErrorMatches, `invalid node affinity constraints: \^=bar`)
}
