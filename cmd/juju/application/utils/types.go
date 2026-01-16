// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

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
	Track  string
	Risk   string
	Branch string
}

// Platform describes the platform used to install the charm with.
type Platform struct {
	Architecture string
	// TODO: This should be of type ostype.OSType
	OS      string
	Channel string
}

// Schema represents the source of the charm.
type Schema string

const (
	// Local represents a local charm.
	Local Schema = "local"
	// CharmHub represents a charm from the new charmHub.
	CharmHub Schema = "charm-hub"
)
