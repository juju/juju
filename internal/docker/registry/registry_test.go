// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
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
