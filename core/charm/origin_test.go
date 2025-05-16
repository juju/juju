// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type sourceSuite struct {
	testhelpers.IsolationSuite
}

func TestSourceSuite(t *stdtesting.T) { tc.Run(t, &sourceSuite{}) }
func (s *sourceSuite) TestMatches(c *tc.C) {
	ok := charm.Source("xxx").Matches("xxx")
	c.Assert(ok, tc.IsTrue)
}

func (s *sourceSuite) TestNotMatches(c *tc.C) {
	ok := charm.Source("xxx").Matches("yyy")
	c.Assert(ok, tc.IsFalse)
}

type platformSuite struct {
	testhelpers.IsolationSuite
}

func TestPlatformSuite(t *stdtesting.T) { tc.Run(t, &platformSuite{}) }
func (s *platformSuite) TestParsePlatform(c *tc.C) {
	tests := []struct {
		Name        string
		Value       string
		Expected    charm.Platform
		ExpectedErr string
	}{{
		Name:        "empty",
		Value:       "",
		ExpectedErr: "platform cannot be empty",
	}, {
		Name:        "empty components",
		Value:       "//",
		ExpectedErr: `architecture in platform "//" not valid`,
	}, {
		Name:        "too many components",
		Value:       "////",
		ExpectedErr: `platform is malformed; it has an invalid number of components "////"`,
	}, {
		Name:        "architecture and channel, no os name",
		Value:       "amd64/18.04",
		ExpectedErr: `platform is malformed; it has an invalid number of components "amd64/18.04"`,
	}, {
		Name:  "architecture",
		Value: "amd64",
		Expected: charm.Platform{
			Architecture: "amd64",
		},
	}, {
		Name:  "architecture, os and series",
		Value: "amd64/os/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "os",
			Channel:      "series",
		},
	}, {
		Name:  "architecture, os, version and risk",
		Value: "amd64/os/version/risk",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "os",
			Channel:      "version/risk",
		},
	}, {
		Name:  "architecture, unknown os and series",
		Value: "amd64/unknown/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Channel:      "series",
		},
	}, {
		Name:  "architecture, unknown os and unknown series",
		Value: "amd64/unknown/unknown",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Channel:      "",
		},
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch, err := charm.ParsePlatformNormalize(test.Value)
		if test.ExpectedErr != "" {
			c.Check(err, tc.ErrorMatches, test.ExpectedErr)
		} else {
			c.Check(ch, tc.DeepEquals, test.Expected)
			c.Check(err, tc.IsNil)
		}
	}
}

func (s *platformSuite) TestString(c *tc.C) {
	tests := []struct {
		Name     string
		Value    string
		Expected string
	}{{
		Name:     "architecture",
		Value:    "amd64",
		Expected: "amd64",
	}, {
		Name:     "architecture, os and version",
		Value:    "amd64/os/version",
		Expected: "amd64/os/version",
	}, {
		Name:     "architecture, os, version and risk",
		Value:    "amd64/os/version/risk",
		Expected: "amd64/os/version/risk",
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		platform, err := charm.ParsePlatformNormalize(test.Value)
		c.Assert(err, tc.IsNil)
		c.Assert(platform.String(), tc.DeepEquals, test.Expected)
	}
}
