// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type clientNormalizeOriginSuite struct {
	testing.IsolationSuite
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

type clientValidateOriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&clientValidateOriginSuite{})

func (s *clientValidateOriginSuite) TestValidateOrigin(c *gc.C) {
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Architecture: "all",
	}
	schema := "ch"

	err := validateOrigin(origin, schema, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientValidateOriginSuite) TestValidateOriginWithEmptyArch(c *gc.C) {
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Architecture: "",
	}
	schema := "ch"

	err := validateOrigin(origin, schema, false)
	c.Assert(err, gc.ErrorMatches, "empty architecture not valid")
}

func (s *clientValidateOriginSuite) TestValidateOriginWithInvalidCharmStoreSource(c *gc.C) {
	origin := params.CharmOrigin{
		Source:       "charm-store",
		Architecture: "all",
	}
	schema := "ch"

	err := validateOrigin(origin, schema, false)
	c.Assert(err, gc.ErrorMatches, `origin source "charm-store" with schema not valid`)
}

func (s *clientValidateOriginSuite) TestValidateOriginWithInvalidCharmHubSource(c *gc.C) {
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Architecture: "all",
	}
	schema := "cs"

	err := validateOrigin(origin, schema, false)
	c.Assert(err, gc.ErrorMatches, `origin source "charm-hub" with schema not valid`)
}

func (s *clientValidateOriginSuite) TestValidateOriginWhenSwitchingCharmsFromDifferentStores(c *gc.C) {
	origin := params.CharmOrigin{
		Source:       "charm-store",
		Architecture: "all",
	}
	schema := "ch"

	err := validateOrigin(origin, schema, true)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("expected validateOrigin to succeed when switching from charmstore to charmhub"))
}
