// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resources"
)

type ResourceSuite struct{}

var _ = gc.Suite(&ResourceSuite{})

func (s *ResourceSuite) TestValidRegistryPath(c *gc.C) {
	err := resources.ValidateDockerRegistryPath("registry.hub.docker.com/me/awesomeimage@sha256:deedbeaf")
	c.Assert(err, jc.ErrorIsNil)
	err = resources.ValidateDockerRegistryPath("docker.io/me/mygitlab:latest")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ResourceSuite) TestInvalidRegistryPath(c *gc.C) {
	err := resources.ValidateDockerRegistryPath("sha256:deedbeaf")
	c.Assert(err, gc.ErrorMatches, "docker image path .* not valid")
}
