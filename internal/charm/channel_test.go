// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type channelSuite struct {
	testhelpers.IsolationSuite
}

func TestChannelSuite(t *stdtesting.T) {
	tc.Run(t, &channelSuite{})
}

func (s *channelSuite) TestParseChannelNormalize(c *tc.C) {
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
			c.Assert(err, tc.ErrorMatches, test.ExpectedErr)
		} else {
			c.Assert(ch, tc.DeepEquals, test.Expected)
			c.Assert(err, tc.IsNil)
		}
	}
}

func (s *channelSuite) TestString(c *tc.C) {
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
		c.Assert(err, tc.IsNil)
		c.Assert(ch.String(), tc.DeepEquals, test.Expected)
	}
}

func (s *channelSuite) TestMakeChannel(c *tc.C) {
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
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(ch, tc.DeepEquals, charm.Channel{
				Track:  test.Track,
				Risk:   charm.Risk(test.Risk),
				Branch: test.Branch,
			})
		} else {
			c.Assert(err, tc.ErrorIs, errors.NotValid)
		}
	}
}

func (s *channelSuite) TestMakePermissiveChannelAndEmpty(c *tc.C) {
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
		c.Assert(ch.String(), tc.Equals, test.Expected)
	}
}

func (s *channelSuite) TestEmpty(c *tc.C) {
	c.Assert(charm.Channel{}.Empty(), tc.IsTrue)
}
