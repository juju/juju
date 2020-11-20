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
	ID string
	// Hash is the hash of the charm intended to be used.
	Hash string
	// Risk is the CharmHub channel risk, or the CharmStore channel value.
	Risk string
	// Revision is the charm revision number.
	Revision *int
	// Track is a CharmHub channel track.
	Track *string
	// Architecture describes the architecture intended to be used by the charm.
	Architecture string
	// OS describes the OS intended to be used by the charm.
	OS string
	// Series describes the series of the OS intended to be used by the charm.
	Series string
}

// CoreChannel returns the core charm channel.
func (o Origin) CoreChannel() corecharm.Channel {
	var track string
	if o.Track != nil {
		track = *o.Track
	}
	return corecharm.Channel{
		Track: track,
		Risk:  corecharm.Risk(o.Risk),
	}
}

// ParamsCharmOrigin is a helper method to get a params version
// of this structure.
func (o Origin) ParamsCharmOrigin() params.CharmOrigin {
	return params.CharmOrigin{
		Source:       o.Source.String(),
		ID:           o.ID,
		Hash:         o.Hash,
		Revision:     o.Revision,
		Risk:         o.Risk,
		Track:        o.Track,
		Architecture: o.Architecture,
		OS:           o.OS,
		Series:       o.Series,
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
		Platform: corecharm.Platform{
			Architecture: o.Architecture,
			OS:           o.OS,
			Series:       o.Series,
		},
	}
}

// APICharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func APICharmOrigin(origin params.CharmOrigin) Origin {
	return Origin{
		Source:       OriginSource(origin.Source),
		ID:           origin.ID,
		Hash:         origin.Hash,
		Risk:         origin.Risk,
		Revision:     origin.Revision,
		Track:        origin.Track,
		Architecture: origin.Architecture,
		OS:           origin.OS,
		Series:       origin.Series,
	}
}

// CoreCharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func CoreCharmOrigin(origin corecharm.Origin) Origin {
	var ch corecharm.Channel
	if origin.Channel != nil {
		ch = *origin.Channel
	}
	var track *string
	if ch.Track != "" {
		track = &ch.Track
	}
	return Origin{
		Source:       OriginSource(origin.Source),
		ID:           origin.ID,
		Hash:         origin.Hash,
		Revision:     origin.Revision,
		Risk:         string(ch.Risk),
		Track:        track,
		Architecture: origin.Platform.Architecture,
		OS:           origin.Platform.OS,
		Series:       origin.Platform.Series,
	}
}
