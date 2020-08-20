// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

// CharmOriginSource represents the source of the charm.
type CharmOriginSource string

func (c CharmOriginSource) String() string {
	return string(c)
}

const (
	// OriginLocal represents a local charm.
	OriginLocal CharmOriginSource = "local"
	// OriginCharmStore represents a charm from the now old charm-store.
	OriginCharmStore CharmOriginSource = "charm-store"
	// OriginCharmHub represents a charm from the new charm-hub.
	OriginCharmHub CharmOriginSource = "charm-hub"
)

// CharmOrigin holds the information about where the charm originates.
type CharmOrigin struct {
	Source   CharmOriginSource
	ID       string
	Hash     string
	Risk     string
	Revision *int
	Channel  *string
}
