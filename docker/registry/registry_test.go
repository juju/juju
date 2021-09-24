// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
)

type registrySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&registrySuite{})

func (s *registrySuite) TestNewRegistryNotSupported(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:    "public.ecr.aws/repo-alias",
		ServerAddress: "public.ecr.aws",
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, gc.ErrorMatches, `container registry "public.ecr.aws" not supported`)
}
