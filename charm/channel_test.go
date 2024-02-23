// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

type channelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&channelSuite{})

func (s channelSuite) TestParseChannelNormalize(c *gc.C) {
	// ParseChannelNormalize tests ParseChannel as well.
	tests := []struct {
		Name        string
		Value       string
		Expected    charm.Channel
		ExpectedErr string
	}{{
		Name:        "empty",
		Value:       "",
		ExpectedErr: "empty channel not valid",
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
		ch, err := charm.ParseChannelNormalize(test.Value)
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
		Expected: "latest/stable",
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
	}, {
		Name:     "latest, risk and branch",
		Value:    "latest/edge/foo",
		Expected: "latest/edge/foo",
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch, err := charm.ParseChannelNormalize(test.Value)
		c.Assert(err, gc.IsNil)
		c.Assert(ch.String(), gc.DeepEquals, test.Expected)
	}
}

func (s channelSuite) TestMakeChannel(c *gc.C) {
	tests := []struct {
		Name      string
		Track     string
		Risk      string
		Branch    string
		Expected  string
		ErrorType func(err error) bool
	}{{
		Name:      "track, risk, branch not normalized",
		Track:     "latest",
		Risk:      "beta",
		Branch:    "bar",
		Expected:  "latest/beta/bar",
		ErrorType: nil,
	}, {
		Name:      "",
		Track:     "",
		Risk:      "testme",
		Branch:    "",
		ErrorType: errors.IsNotValid,
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch, err := charm.MakeChannel(test.Track, test.Risk, test.Branch)
		if test.ErrorType == nil {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(ch, gc.DeepEquals, charm.Channel{
				Track:  test.Track,
				Risk:   charm.Risk(test.Risk),
				Branch: test.Branch,
			})
		} else {
			c.Assert(err, jc.Satisfies, errors.IsNotValid)
		}
	}
}

func (s channelSuite) TestMakePermissiveChannelAndEmpty(c *gc.C) {
	tests := []struct {
		Name     string
		Track    string
		Risk     string
		Expected string
	}{{
		Name:     "latest track, risk",
		Track:    "latest",
		Risk:     "beta",
		Expected: "latest/beta",
	}, {
		Name:     "risk not valid",
		Track:    "",
		Risk:     "testme",
		Expected: "testme",
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch := charm.MakePermissiveChannel(test.Track, test.Risk, "")
		c.Assert(ch.String(), gc.Equals, test.Expected)
	}
}

func (s channelSuite) TestEmpty(c *gc.C) {
	c.Assert(charm.Channel{}.Empty(), jc.IsTrue)
}
