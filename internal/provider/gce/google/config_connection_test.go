// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/gce/google"
)

type connConfigSuite struct {
	google.BaseSuite
}

func TestConnConfigSuite(t *stdtesting.T) { tc.Run(t, &connConfigSuite{}) }
func (*connConfigSuite) TestValidateValid(c *tc.C) {
	cfg := google.ConnectionConfig{
		Region:     "spam",
		ProjectID:  "eggs",
		HTTPClient: jujuhttp.NewClient(),
	}
	err := cfg.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (*connConfigSuite) TestValidateMissingRegion(c *tc.C) {
	cfg := google.ConnectionConfig{
		ProjectID: "eggs",
	}
	err := cfg.Validate()

	c.Assert(err, tc.FitsTypeOf, &google.InvalidConfigValueError{})
	c.Check(err.(*google.InvalidConfigValueError).Key, tc.Equals, "GCE_REGION")
}

func (*connConfigSuite) TestValidateMissingProjectID(c *tc.C) {
	cfg := google.ConnectionConfig{
		Region: "spam",
	}
	err := cfg.Validate()

	c.Assert(err, tc.FitsTypeOf, &google.InvalidConfigValueError{})
	c.Check(err.(*google.InvalidConfigValueError).Key, tc.Equals, "GCE_PROJECT_ID")
}
