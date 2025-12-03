// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ChannelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ChannelSuite{})

func (s *ChannelSuite) TestParse(c *gc.C) {
	ch, err := ParseChannel("22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04"})
	ch, err = ParseChannel("22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "edge"})
	ch, err = ParseChannel("all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "all"})
}

func (s *ChannelSuite) TestParseError(c *gc.C) {
	_, err := ParseChannel("22.04/edge/foo")
	c.Assert(err, gc.ErrorMatches, `channel is malformed and has too many components "22.04/edge/foo"`)
	_, err = ParseChannel("22.04/foo")
	c.Assert(err, gc.ErrorMatches, `risk in channel "22.04/foo" not valid`)
}

func (s *ChannelSuite) TestParseNormalise(c *gc.C) {
	ch, err := ParseChannelNormalize("22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "stable"})
	ch, err = ParseChannelNormalize("22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "edge"})
}

func (s *ChannelSuite) TestMakeDefaultChannel(c *gc.C) {
	ch := MakeDefaultChannel("22.04")
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "stable"})
}

func (s *ChannelSuite) TestString(c *gc.C) {
	c.Assert(Channel{Track: "22.04"}.String(), gc.Equals, "22.04")
	c.Assert(Channel{Track: "22.04", Risk: "edge"}.String(), gc.Equals, "22.04/edge")
}

func (s *ChannelSuite) TestDisplayString(c *gc.C) {
	c.Assert(Channel{Track: "18.04"}.DisplayString(), gc.Equals, "18.04")
	c.Assert(Channel{Track: "20.04", Risk: "stable"}.DisplayString(), gc.Equals, "20.04")
	c.Assert(Channel{Track: "22.04", Risk: "edge"}.DisplayString(), gc.Equals, "22.04/edge")
}

func (s *ChannelSuite) TestHasHigherPriorityThan(c *gc.C) {
	supportedLTSTrack := DefaultSupportedLTSBase().Channel.Track

	// Split base track to get year and month.
	parts := strings.Split(supportedLTSTrack, ".")
	c.Assert(len(parts), gc.Equals, 2)
	ltsYear, err := strconv.Atoi(parts[0])
	c.Assert(err, jc.ErrorIsNil)

	// Create future LTS base track.
	futureTrack := fmt.Sprintf("%02d.%s", ltsYear+2, parts[1])

	tests := []struct {
		name     string
		current  Channel
		other    Channel
		expected bool
		reason   string
	}{
		{
			name:     "LTS base track has highest priority",
			current:  Channel{Track: supportedLTSTrack, Risk: Stable},
			other:    Channel{Track: futureTrack, Risk: Stable},
			expected: true,
			reason:   "LTS base track should be preferred",
		},
		{
			name:     "non-LTS base track lower priority than LTS",
			current:  Channel{Track: futureTrack, Risk: Stable},
			other:    Channel{Track: supportedLTSTrack, Risk: Stable},
			expected: false,
			reason:   "non-LTS should not be preferred over LTS",
		},
		{
			name:     "higher version base track preferred when neither is LTS",
			current:  Channel{Track: "22.04", Risk: Stable},
			other:    Channel{Track: "20.04", Risk: Stable},
			expected: true,
			reason:   "22.04 > 20.04",
		},
		{
			name:     "lower version base track not preferred when neither is LTS",
			current:  Channel{Track: "20.04", Risk: Stable},
			other:    Channel{Track: "22.04", Risk: Stable},
			expected: false,
			reason:   "20.04 < 22.04",
		},
		{
			name:     "stable risk preferred over candidate for same base track",
			current:  Channel{Track: "22.04", Risk: Stable},
			other:    Channel{Track: "22.04", Risk: Candidate},
			expected: true,
			reason:   "stable > candidate",
		},
		{
			name:     "stable risk preferred over beta for same base track",
			current:  Channel{Track: "22.04", Risk: Stable},
			other:    Channel{Track: "22.04", Risk: Beta},
			expected: true,
			reason:   "stable > beta",
		},
		{
			name:     "stable risk preferred over edge for same base track",
			current:  Channel{Track: "22.04", Risk: Stable},
			other:    Channel{Track: "22.04", Risk: Edge},
			expected: true,
			reason:   "stable > edge",
		},
		{
			name:     "candidate risk preferred over beta for same base track",
			current:  Channel{Track: "22.04", Risk: Candidate},
			other:    Channel{Track: "22.04", Risk: Beta},
			expected: true,
			reason:   "candidate > beta",
		},
		{
			name:     "candidate risk preferred over edge for same base track",
			current:  Channel{Track: "22.04", Risk: Candidate},
			other:    Channel{Track: "22.04", Risk: Edge},
			expected: true,
			reason:   "candidate > edge",
		},
		{
			name:     "beta risk preferred over edge for same base track",
			current:  Channel{Track: "22.04", Risk: Beta},
			other:    Channel{Track: "22.04", Risk: Edge},
			expected: true,
			reason:   "beta > edge",
		},
		{
			name:     "edge risk not preferred over stable for same base track",
			current:  Channel{Track: "22.04", Risk: Edge},
			other:    Channel{Track: "22.04", Risk: Stable},
			expected: false,
			reason:   "edge < stable",
		},
		{
			name:     "base track priority overrides risk, higher base track is preferred even with worse risk",
			current:  Channel{Track: "22.04", Risk: Edge},
			other:    Channel{Track: "20.04", Risk: Stable},
			expected: true,
			reason:   "base track version takes precedence over risk",
		},
		{
			name:     "LTS base track with edge risk preferred over non-LTS with stable risk",
			current:  Channel{Track: supportedLTSTrack, Risk: Edge},
			other:    Channel{Track: futureTrack, Risk: Stable},
			expected: true,
			reason:   "LTS base track priority overrides risk differences",
		},
		{
			name:     "LTS base track with stable risk preferred over LTS with edge risk",
			current:  Channel{Track: supportedLTSTrack, Risk: Stable},
			other:    Channel{Track: supportedLTSTrack, Risk: Edge},
			expected: true,
			reason:   "LTS base track priority overrides risk differences",
		},
		{
			name:     "same base track and risk returns false",
			current:  Channel{Track: "22.04", Risk: Stable},
			other:    Channel{Track: "22.04", Risk: Stable},
			expected: false,
			reason:   "identical channels have no priority difference",
		},
	}

	for _, test := range tests {
		result := test.current.HasHigherPriorityThan(test.other)
		c.Check(result, gc.Equals, test.expected, gc.Commentf("test '%s' failed with reason: %s", test.name, test.reason))
	}
}
