// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
)

type registrySuite struct {
}

var _ = tc.Suite(&registrySuite{})

func (s *registrySuite) TestErrorsOnDockerDefault(c *tc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "jujusolutions",
	})
	c.Assert(err, tc.ErrorMatches, `oci reference "jujusolutions" must have a domain`)
	c.Assert(reg, tc.IsNil)
}

func (s *registrySuite) TestSelectsAWSPrivate(c *tc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "123456.dkr.ecr.eu-west-1.amazonaws.com",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "access key id",
			Password: "secret key",
		},
		Region: "us-west-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reg, tc.NotNil)
	c.Assert(reg.String(), tc.Equals, "*.dkr.ecr.*.amazonaws.com")
}

func (s *registrySuite) TestSelectsDockerHub(c *tc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "docker.io/jujusolutions",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reg, tc.NotNil)
	c.Assert(reg.String(), tc.Equals, "docker.io")
}

func (s *registrySuite) TestSelectsGithubContainerRegistry(c *tc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "ghcr.io/juju",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reg, tc.NotNil)
	c.Assert(reg.String(), tc.Equals, "ghcr.io")
}

func (s *registrySuite) TestSelectsAWSPublic(c *tc.C) {
	reg, err := registry.New(docker.ImageRepoDetails{
		Repository: "public.ecr.aws/juju",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reg, tc.NotNil)
	c.Assert(reg.String(), tc.Equals, "public.ecr.aws")
}
