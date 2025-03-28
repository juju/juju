// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"fmt"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
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

// Channel identifies and describes completely an os channel.
//
// A channel consists of, and is subdivided by, tracks and risk-levels:
//   - Tracks represents the version of the os, eg "22.04".
//   - Risk-levels represent a progressive potential trade-off between stability
//     and new features.
//
// The complete channel name can be structured as three distinct parts separated
// by slashes:
//
//	<track>/<risk>
type Channel struct {
	Track string `json:"track,omitempty"`
	Risk  Risk   `json:"risk,omitempty"`
}

// MakeDefaultChannel creates a normalized channel for
// the specified track with a default risk of "stable".
func MakeDefaultChannel(track string) Channel {
	ch := Channel{
		Track: track,
	}
	return ch.Normalize()
}

// ParseChannel parses a string representing a channel.
func ParseChannel(s string) (Channel, error) {
	if s == "" {
		return Channel{}, errors.Errorf("empty channel %w", coreerrors.NotValid)
	}

	p := strings.Split(s, "/")

	var risk, track *string
	switch len(p) {
	case 1:
		track = &p[0]
	case 2:
		track, risk = &p[0], &p[1]
	default:
		return Channel{}, errors.Errorf("channel is malformed and has too many components %q", s)
	}

	ch := Channel{}

	if risk != nil {
		if !isRisk(*risk) {
			return Channel{}, errors.Errorf("risk in channel %q %w", s, coreerrors.NotValid)
		}
		// We can lift this into a risk, as we've validated prior to this to
		// ensure it's a valid risk.
		ch.Risk = Risk(*risk)
	}
	if track != nil {
		if *track == "" {
			return Channel{}, errors.Errorf("track in channel %q %w", s, coreerrors.NotValid)
		}
		ch.Track = *track
	}
	return ch, nil
}

// ParseChannelNormalize parses a string representing a store channel.
// The returned channel's track, risk and name are normalized.
func ParseChannelNormalize(s string) (Channel, error) {
	ch, err := ParseChannel(s)
	if err != nil {
		return Channel{}, errors.Capture(err)
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
		Track: track,
		Risk:  risk,
	}
}

// Empty returns true if all it's components are empty.
func (ch Channel) Empty() bool {
	return ch.Track == "" && ch.Risk == ""
}

func (ch Channel) String() string {
	path := ch.Track
	if risk := ch.Risk; risk != "" {
		path = fmt.Sprintf("%s/%s", path, risk)
	}
	return path
}

func (ch Channel) DisplayString() string {
	track, risk := ch.Track, ch.Risk
	if risk == Stable {
		risk = ""
	}
	if risk == "" {
		return track
	}
	return fmt.Sprintf("%s/%s", track, risk)
}
