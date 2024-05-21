// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// Risk describes the type of risk in a current channel.
type Risk string

const (
	Stable    Risk = "stable"
	Candidate Risk = "candidate"
	Beta      Risk = "beta"
	Edge      Risk = "edge"
)

// Risks is a list of the available channel risks.
var Risks = []Risk{
	Stable,
	Candidate,
	Beta,
	Edge,
}

func isRisk(potential string) bool {
	for _, risk := range Risks {
		if potential == string(risk) {
			return true
		}
	}
	return false
}

// Channel identifies and describes completely a store channel.
//
// A channel consists of, and is subdivided by, tracks, risk-levels and
// branches:
//   - Tracks enable snap developers to publish multiple supported releases of
//     their application under the same snap name.
//   - Risk-levels represent a progressive potential trade-off between stability
//     and new features.
//   - Branches are _optional_ and hold temporary releases intended to help with
//     bug-fixing.
//
// The complete channel name can be structured as three distinct parts separated
// by slashes:
//
//	<track>/<risk>/<branch>
type Channel struct {
	Track  string `json:"track,omitempty"`
	Risk   Risk   `json:"risk,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// MakeChannel creates a core charm Channel from a set of component parts.
func MakeChannel(track, risk, branch string) (Channel, error) {
	if !isRisk(risk) {
		return Channel{}, errors.NotValidf("risk %q", risk)
	}
	return Channel{
		Track:  track,
		Risk:   Risk(risk),
		Branch: branch,
	}, nil
}

// MakePermissiveChannel creates a normalized core charm channel which
// never fails.  It assumes that the risk has been prechecked.
func MakePermissiveChannel(track, risk, branch string) Channel {
	ch := Channel{
		Track:  track,
		Risk:   Risk(risk),
		Branch: branch,
	}
	return ch.Normalize()
}

// ParseChannel parses a string representing a store channel.
func ParseChannel(s string) (Channel, error) {
	if s == "" {
		return Channel{}, errors.NotValidf("empty channel")
	}

	p := strings.Split(s, "/")

	var risk, track, branch *string
	switch len(p) {
	case 1:
		if isRisk(p[0]) {
			risk = &p[0]
		} else {
			track = &p[0]
		}
	case 2:
		if isRisk(p[0]) {
			risk, branch = &p[0], &p[1]
		} else {
			track, risk = &p[0], &p[1]
		}
	case 3:
		track, risk, branch = &p[0], &p[1], &p[2]
	default:
		return Channel{}, errors.Errorf("channel is malformed and has too many components %q", s)
	}

	ch := Channel{}

	if risk != nil {
		if !isRisk(*risk) {
			return Channel{}, errors.NotValidf("risk in channel %q", s)
		}
		// We can lift this into a risk, as we've validated prior to this to
		// ensure it's a valid risk.
		ch.Risk = Risk(*risk)
	}
	if track != nil {
		if *track == "" {
			return Channel{}, errors.NotValidf("track in channel %q", s)
		}
		ch.Track = *track
	}
	if branch != nil {
		if *branch == "" {
			return Channel{}, errors.NotValidf("branch in channel %q", s)
		}
		ch.Branch = *branch
	}
	return ch, nil
}

// ParseChannelNormalize parses a string representing a store channel.
// The returned channel's track, risk and name are normalized.
func ParseChannelNormalize(s string) (Channel, error) {
	ch, err := ParseChannel(s)
	if err != nil {
		return Channel{}, errors.Trace(err)
	}
	return ch.Normalize(), nil
}

// Normalize the channel with normalized track, risk and names.
func (ch Channel) Normalize() Channel {
	track := ch.Track

	risk := ch.Risk
	if risk == "" {
		risk = "stable"
	}

	return Channel{
		Track:  track,
		Risk:   risk,
		Branch: ch.Branch,
	}
}

// Empty returns true if all it's components are empty.
func (ch Channel) Empty() bool {
	return ch.Track == "" && ch.Risk == "" && ch.Branch == ""
}

func (ch Channel) String() string {
	path := string(ch.Risk)
	if track := ch.Track; track != "" {
		path = fmt.Sprintf("%s/%s", track, path)
	}
	if branch := ch.Branch; branch != "" {
		path = fmt.Sprintf("%s/%s", path, branch)
	}

	return path
}
