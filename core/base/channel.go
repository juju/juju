// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// DefaultSupportedLTSBase returns the latest LTS base that Juju supports
// and is compatible with.
func DefaultSupportedLTSBase() Base {
	return MakeDefaultBase(UbuntuOS, "24.04")
}

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

// isMoreStableThan returns true if r is more stable than other.
// Stability order: stable > candidate > beta > edge.
func (r Risk) isMoreStableThan(other Risk) bool {
	return r.index() < other.index()
}

// index returns the stability index of a risk.
// Lower index means more stable.
func (r Risk) index() int {
	switch r {
	case Stable:
		return 0
	case Candidate:
		return 1
	case Beta:
		return 2
	case Edge:
		return 3
	default:
		return 4 // This should not happen, but unknown risks are least stable.
	}
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
		return Channel{}, errors.NotValidf("empty channel")
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
	if risk == "" && track != "kubernetes" {
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
	if risk == Stable && track != "kubernetes" {
		risk = ""
	}
	if risk == "" {
		return track
	}
	return fmt.Sprintf("%s/%s", track, risk)
}

// HasHigherPriorityThan returns true if this channel has higher priority than the other channel.
// Preference order:
// 1. Supported LTS track has highest priority
// 2. Tracks in descending order (higher version preferred)
// 3. Risk stability (stable > candidate > beta > edge) if tracks are equal
func (ch Channel) HasHigherPriorityThan(other Channel) bool {
	// First priority: Is current ch the LTS? eg. If 24.04 is the supported LTS, prefer track 24.04.
	supportedLTSTrack := DefaultSupportedLTSBase().Channel.Track
	chIsLTSTrack := ch.Track == supportedLTSTrack
	otherIsLTSTrack := other.Track == supportedLTSTrack
	if chIsLTSTrack != otherIsLTSTrack {
		return chIsLTSTrack
	}

	// Second priority: Track version (descending), eg. 22.04 > 20.04.
	if ch.Track != other.Track {
		return ch.Track > other.Track
	}

	// Third priority: Risk stability, eg. stable > candidate > beta > edge.
	return ch.Risk.isMoreStableThan(other.Risk)
}
