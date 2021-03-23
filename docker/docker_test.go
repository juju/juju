// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"time"

	"github.com/juju/juju/tools"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type DockerSuiteSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&DockerSuiteSuite{})

func (s *DockerSuiteSuite) TestListImages(c *gc.C) {
	s.PatchValue(&HttpGet, func(url string, timeout time.Duration) ([]byte, error) {
		c.Assert(url, gc.Equals, "https://registry.hub.docker.com/v1/repositories/path/tags")
		c.Assert(timeout, gc.Equals, 30*time.Second)
		return []byte(`[{"name": "2.6.0"}, {"name": "2.6-beta1"}, {"name": "bad"}]`), nil
	})
	v, err := ListOperatorImages("path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, jc.DeepEquals, tools.Versions{
		imageInfo{version: version.MustParse("2.6.0")},
		imageInfo{version: version.MustParse("2.6-beta1")},
	})
}
