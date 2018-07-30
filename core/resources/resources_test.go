// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"encoding/json"

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

func (s *ResourceSuite) TestDockerImageDetailsUnmarshal(c *gc.C) {
	data := []byte(`{"ImageName":"testing@sha256:beef-deed","Username":"docker-registry","Password":"fragglerock"}`)
	var result resources.DockerImageDetails
	err := json.Unmarshal(data, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, resources.DockerImageDetails{
		RegistryPath: "testing@sha256:beef-deed",
		Username:     "docker-registry",
		Password:     "fragglerock",
	})
}
