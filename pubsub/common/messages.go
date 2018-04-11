// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// OriginTarget represents the commonly used message structure
// where the publisher generally just specifies the Target and the
// Origin is filled in by the hub.
type OriginTarget struct {
	// Origin represents this API server.
	Origin string `yaml:"origin"`
	// Target represents the other API server that this one is forwarding
	// messages to.
	Target string `yaml:"target"`
}
