// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/juju/apiserver/params"
	corecharm "github.com/juju/juju/core/charm"
)

// OriginSource represents the source of the charm.
type OriginSource string

func (c OriginSource) String() string {
	return string(c)
}

const (
	// OriginLocal represents a local charm.
	OriginLocal OriginSource = "local"
	// OriginCharmStore represents a charm from the now old charm-store.
	OriginCharmStore OriginSource = "charm-store"
	// OriginCharmHub represents a charm from the new charm-hub.
	OriginCharmHub OriginSource = "charm-hub"
)

// Origin holds the information about where the charm originates.
type Origin struct {
	// Source is where the charm came from, Local, CharmStore or CharmHub.
	Source OriginSource
	// ID is the CharmHub ID for this charm.
	ID   string
	Hash string
	// Risk is the CharmHub channel risk, or the CharmStore channel value.
	Risk string
	// Revision is the charm revision number.
	Revision *int
	// Track is a CharmHub channel track.
	Track *string
}

// ParamsCharmOrigin is a helper method to get a params version
// of this structure.
func (o Origin) ParamsCharmOrigin() params.CharmOrigin {
	return params.CharmOrigin{
		Source:   o.Source.String(),
		ID:       o.ID,
		Hash:     o.Hash,
		Revision: o.Revision,
		Risk:     o.Risk,
		Track:    o.Track,
	}
}

// CoreCharmOrigin is a help method to get a core version of this structure.
func (o Origin) CoreCharmOrigin() corecharm.Origin {
	var track string
	if o.Track != nil {
		track = *o.Track
	}
	return corecharm.Origin{
		Source:   corecharm.Source(o.Source),
		ID:       o.ID,
		Hash:     o.Hash,
		Revision: o.Revision,
		Channel: &corecharm.Channel{
			Risk:  corecharm.Risk(o.Risk),
			Track: track,
		},
	}
}

// APICharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func APICharmOrigin(origin params.CharmOrigin) Origin {
	return Origin{
		Source:   OriginSource(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Risk:     origin.Risk,
		Revision: origin.Revision,
		Track:    origin.Track,
	}
}
