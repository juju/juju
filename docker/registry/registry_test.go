// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
)

type registrySuite struct {
}

var _ = gc.Suite(&registrySuite{})

func (s *registrySuite) TestErrorsOnDockerDefault(c *gc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "jujusolutions",
	})
	c.Assert(err, gc.ErrorMatches, `oci reference "jujusolutions" must have a domain`)
	c.Assert(reg, gc.IsNil)
}

func (s *registrySuite) TestSelectsAWSPrivate(c *gc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "123456.dkr.ecr.eu-west-1.amazonaws.com",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "access key id",
			Password: "secret key",
		},
		Region: "us-west-1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reg, gc.NotNil)
	c.Assert(reg.String(), gc.Equals, "*.dkr.ecr.*.amazonaws.com")
}

func (s *registrySuite) TestSelectsDockerHub(c *gc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "ghcr.io/juju",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reg, gc.NotNil)
	c.Assert(reg.String(), gc.Equals, "docker.io")
}

func (s *registrySuite) TestSelectsGithubContainerRegistry(c *gc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "ghcr.io/juju",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reg, gc.NotNil)
	c.Assert(reg.String(), gc.Equals, "ghcr.io")
}

func (s *registrySuite) TestSelectsAWSPublic(c *gc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "public.ecr.aws/juju",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reg, gc.NotNil)
	c.Assert(reg.String(), gc.Equals, "public.ecr.aws")
}
