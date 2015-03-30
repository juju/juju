// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type connConfigSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&connConfigSuite{})

func (*connConfigSuite) TestValidateValid(c *gc.C) {
	cfg := google.ConnectionConfig{
		Region:    "spam",
		ProjectID: "eggs",
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*connConfigSuite) TestValidateMissingRegion(c *gc.C) {
	cfg := google.ConnectionConfig{
		ProjectID: "eggs",
	}
	err := cfg.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidConfigValue{})
	c.Check(err.(*google.InvalidConfigValue).Key, gc.Equals, "GCE_REGION")
}

func (*connConfigSuite) TestValidateMissingProjectID(c *gc.C) {
	cfg := google.ConnectionConfig{
		Region: "spam",
	}
	err := cfg.Validate()

	c.Assert(err, gc.FitsTypeOf, &google.InvalidConfigValue{})
	c.Check(err.(*google.InvalidConfigValue).Key, gc.Equals, "GCE_PROJECT_ID")
}
