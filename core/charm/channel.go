// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
)

const (
	// DefaultChannelString represents the default track and risk if nothing
	// is found.
	DefaultChannelString = "stable"
)

var (
	// DefaultChannel represents the default track and risk.
	DefaultChannel = charm.Channel{
		Risk: charm.Stable,
	}
	// DefaultRiskChannel represents the default only risk channel.
	DefaultRiskChannel = charm.Channel{
		Risk: charm.Stable,
	}
)

// MakeRiskOnlyChannel creates a charm channel that is backwards compatible with
// old style charm store channels. This creates a risk aware channel only.
// No validation is performed on the risk and is just accepted as is.
func MakeRiskOnlyChannel(risk string) charm.Channel {
	return charm.Channel{
		Risk: charm.Risk(risk),
	}
}

// MustParseChannel parses a given string or returns a panic.
func MustParseChannel(s string) charm.Channel {
	c, err := charm.ParseChannelNormalize(s)
	if err != nil {
		panic(err)
	}
	return c
}
