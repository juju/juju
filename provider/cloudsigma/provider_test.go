// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"flag"
	"testing"

	gc "gopkg.in/check.v1"

	tt "github.com/juju/juju/testing"
)

var live = flag.Bool("live", false, "run tests on live CloudSigma account")

func TestCloudSigma(t *testing.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	tt.BaseSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestProviderBoilerplateConfig(c *gc.C) {
	cfg := providerInstance.BoilerplateConfig()
	c.Assert(cfg, gc.Not(gc.Equals), "")
}
