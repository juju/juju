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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reg, gc.IsNil)
}
