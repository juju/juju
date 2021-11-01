// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v8"

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
	// Type defines the charm type if it's a bundle or a charm
	Type string
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

	// InstanceKey is a unique string associated with the application. To
	// assist with keeping KPI data in charmhub, it must be the same for every
	// charmhub Refresh action related to an application. Create with the
	// charmhub.CreateInstanceKey method. LP: 1944582
	InstanceKey string
}

// WithSeries allows to update the series of an origin.
func (o Origin) WithSeries(series string) Origin {
	other := o
	other.Series = series
	return other
}

// CharmChannel returns the the channel indicated by this origin.
func (o Origin) CharmChannel() charm.Channel {
	var track string
	if o.Track != nil {
		track = *o.Track
	}
	return charm.Channel{
		Track: track,
		Risk:  charm.Risk(o.Risk),
	}
}

// ParamsCharmOrigin is a helper method to get a params version
// of this structure.
func (o Origin) ParamsCharmOrigin() params.CharmOrigin {
	return params.CharmOrigin{
		Source:       o.Source.String(),
		Type:         o.Type,
		ID:           o.ID,
		Hash:         o.Hash,
		Revision:     o.Revision,
		Risk:         o.Risk,
		Track:        o.Track,
		Architecture: o.Architecture,
		OS:           o.OS,
		Series:       o.Series,
		InstanceKey:  o.InstanceKey,
	}
}

// CoreCharmOrigin is a help method to get a core version of this structure.
func (o Origin) CoreCharmOrigin() corecharm.Origin {
	var track string
	if o.Track != nil {
		track = *o.Track
	}
	var channel *charm.Channel
	if o.Risk != "" {
		channel = &charm.Channel{
			Risk:  charm.Risk(o.Risk),
			Track: track,
		}
	}
	return corecharm.Origin{
		Source:   corecharm.Source(o.Source),
		Type:     o.Type,
		ID:       o.ID,
		Hash:     o.Hash,
		Revision: o.Revision,
		Channel:  channel,
		Platform: corecharm.Platform{
			Architecture: o.Architecture,
			OS:           o.OS,
			Series:       o.Series,
		},
		InstanceKey: o.InstanceKey,
	}
}

// APICharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func APICharmOrigin(origin params.CharmOrigin) Origin {
	return Origin{
		Source:       OriginSource(origin.Source),
		Type:         origin.Type,
		ID:           origin.ID,
		Hash:         origin.Hash,
		Risk:         origin.Risk,
		Revision:     origin.Revision,
		Track:        origin.Track,
		Architecture: origin.Architecture,
		OS:           origin.OS,
		Series:       origin.Series,
		InstanceKey:  origin.InstanceKey,
	}
}

// CoreCharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func CoreCharmOrigin(origin corecharm.Origin) Origin {
	var ch charm.Channel
	if origin.Channel != nil {
		ch = *origin.Channel
	}
	var track *string
	if ch.Track != "" {
		track = &ch.Track
	}
	return Origin{
		Source:       OriginSource(origin.Source),
		Type:         origin.Type,
		ID:           origin.ID,
		Hash:         origin.Hash,
		Revision:     origin.Revision,
		Risk:         string(ch.Risk),
		Track:        track,
		Architecture: origin.Platform.Architecture,
		OS:           origin.Platform.OS,
		Series:       origin.Platform.Series,
		InstanceKey:  origin.InstanceKey,
	}
}
