// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type clientNormalizeOriginSuite struct {
}

var _ = gc.Suite(&clientNormalizeOriginSuite{})

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginNoAll(c *gc.C) {
	track := "1.0"
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Type:         "charm",
		Risk:         "edge",
		Track:        &track,
		Architecture: "all",
		OS:           "all",
		Series:       "all",
	}
	obtained, err := normalizeCharmOrigin(origin, "amd64")
	c.Assert(err, jc.ErrorIsNil)
	origin.Architecture = "amd64"
	origin.OS = ""
	origin.Series = ""
	c.Assert(obtained, gc.DeepEquals, origin)
}

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginWithEmpty(c *gc.C) {
	track := "1.0"
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Type:         "charm",
		Risk:         "edge",
		Track:        &track,
		Architecture: "",
		OS:           "all",
		Series:       "all",
	}
	obtained, err := normalizeCharmOrigin(origin, "amd64")
	c.Assert(err, jc.ErrorIsNil)
	origin.Architecture = "amd64"
	origin.OS = ""
	origin.Series = ""
	c.Assert(obtained, gc.DeepEquals, origin)
}

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginLowerCase(c *gc.C) {
	track := "1.0"
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Type:         "charm",
		Risk:         "edge",
		Track:        &track,
		Architecture: "s390",
		OS:           "Ubuntu",
		Series:       "focal",
	}
	obtained, err := normalizeCharmOrigin(origin, "amd64")
	c.Assert(err, jc.ErrorIsNil)
	origin.OS = "ubuntu"
	c.Assert(obtained, gc.DeepEquals, origin)
}
