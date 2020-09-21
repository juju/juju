// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/charm"
)

type channelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&channelSuite{})

func (s channelSuite) TestParseChannel(c *gc.C) {
	tests := []struct {
		Name        string
		Value       string
		Expected    charm.Channel
		ExpectedErr string
	}{{
		Name:        "empty",
		Value:       "",
		ExpectedErr: "channel cannot be empty",
	}, {
		Name:        "empty components",
		Value:       "//",
		ExpectedErr: `risk in channel "//" not valid`,
	}, {
		Name:        "too many components",
		Value:       "////",
		ExpectedErr: `channel is malformed and has too many components "////"`,
	}, {
		Name:        "invalid risk",
		Value:       "track/meshuggah",
		ExpectedErr: `risk in channel "track/meshuggah" not valid`,
	}, {
		Name:  "risk",
		Value: "stable",
		Expected: charm.Channel{
			Risk: "stable",
		},
	}, {
		Name:  "track",
		Value: "meshuggah",
		Expected: charm.Channel{
			Track: "meshuggah",
			Risk:  "stable",
		},
	}, {
		Name:  "risk and branch",
		Value: "stable/foo",
		Expected: charm.Channel{
			Risk:   "stable",
			Branch: "foo",
		},
	}, {
		Name:  "track and risk",
		Value: "foo/stable",
		Expected: charm.Channel{
			Track: "foo",
			Risk:  "stable",
		},
	}, {
		Name:  "track, risk and branch",
		Value: "foo/stable/bar",
		Expected: charm.Channel{
			Track:  "foo",
			Risk:   "stable",
			Branch: "bar",
		},
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch, err := charm.ParseChannel(test.Value)
		if test.ExpectedErr != "" {
			c.Assert(err, gc.ErrorMatches, test.ExpectedErr)
		} else {
			c.Assert(ch, gc.DeepEquals, test.Expected)
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s channelSuite) TestString(c *gc.C) {
	tests := []struct {
		Name     string
		Value    string
		Expected string
	}{{
		Name:     "risk",
		Value:    "stable",
		Expected: "stable",
	}, {
		Name:     "latest track",
		Value:    "latest/stable",
		Expected: "stable",
	}, {
		Name:     "track",
		Value:    "1.0",
		Expected: "1.0/stable",
	}, {
		Name:     "track and risk",
		Value:    "1.0/edge",
		Expected: "1.0/edge",
	}, {
		Name:     "track, risk and branch",
		Value:    "1.0/edge/foo",
		Expected: "1.0/edge/foo",
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch, err := charm.ParseChannel(test.Value)
		c.Assert(err, gc.IsNil)
		c.Assert(ch.String(), gc.DeepEquals, test.Expected)
	}
}
