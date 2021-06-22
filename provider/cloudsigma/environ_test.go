// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	stdcontext "context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/testing"
)

var _ environs.Environ = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)
var _ simplestreams.AgentMetadataValidator = (*environ)(nil)
var _ simplestreams.ImageMetadataValidator = (*environ)(nil)

type environSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *environSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&newClient, func(environscloudspec.CloudSpec, string) (*environClient, error) {
		return nil, nil
	})
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) TestBase(c *gc.C) {
	baseConfig := newConfig(c, validAttrs().Merge(testing.Attrs{"name": "testname"}))
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: baseConfig,
	})
	c.Assert(err, gc.IsNil)

	cfg := env.Config()
	c.Assert(cfg, gc.NotNil)
	c.Check(cfg.Name(), gc.Equals, "testname")

	c.Check(env.PrecheckInstance(context.NewEmptyCloudCallContext(), environs.PrecheckInstanceParams{}), gc.IsNil)

	hasRegion, ok := env.(simplestreams.HasRegion)
	c.Check(ok, gc.Equals, true)
	c.Assert(hasRegion, gc.NotNil)

	cloudSpec, err := hasRegion.Region()
	c.Assert(err, gc.IsNil)
	c.Check(cloudSpec.Region, gc.Not(gc.Equals), "")
	c.Check(cloudSpec.Endpoint, gc.Not(gc.Equals), "")

}

func (s *environSuite) TestUnsupportedConstraints(c *gc.C) {
	baseConfig := newConfig(c, validAttrs().Merge(testing.Attrs{"name": "testname"}))
	env, err := environs.New(stdcontext.TODO(), environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: baseConfig,
	})
	c.Assert(err, gc.IsNil)

	validator, err := env.ConstraintsValidator(context.NewEmptyCloudCallContext())
	c.Assert(err, gc.IsNil)
	c.Check(validator, gc.NotNil)

	unsupported, err := validator.Validate(constraints.MustParse(
		"arch=amd64 tags=foo cpu-power=100 virt-type=kvm",
	))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}
