// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker_test

import (
	"time"

	"github.com/juju/juju/tools"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
	coretesting "github.com/juju/juju/testing"
)

type DockerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&DockerSuite{})

func (s *DockerSuite) TestListImages(c *gc.C) {
	s.PatchValue(&docker.HttpGet, func(url string, timeout time.Duration) ([]byte, error) {
		c.Assert(url, gc.Equals, "https://registry.hub.docker.com/v1/repositories/path/tags")
		c.Assert(timeout, gc.Equals, 30*time.Second)
		return []byte(`[{"name": "2.6.0"}, {"name": "2.6-beta1"}, {"name": "bad"}]`), nil
	})
	v, err := docker.ListOperatorImages("path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, jc.DeepEquals, tools.Versions{
		docker.NewImageInfo(version.MustParse("2.6.0")),
		docker.NewImageInfo(version.MustParse("2.6-beta1")),
	})
}
