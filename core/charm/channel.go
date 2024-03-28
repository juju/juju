// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v12"
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

// MustParseChannel parses a given string or returns a panic.
func MustParseChannel(s string) charm.Channel {
	c, err := charm.ParseChannelNormalize(s)
	if err != nil {
		panic(err)
	}
	return c
}
