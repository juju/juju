// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v10"
	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
)

// OriginSource represents the source of the charm.
type OriginSource string

func (c OriginSource) String() string {
	return string(c)
}

const (
	// OriginLocal represents a local charm.
	OriginLocal OriginSource = "local"
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
	// Branch is the CharmHub channel branch
	Branch *string
	// Architecture describes the architecture intended to be used by the charm.
	Architecture string
	// Base describes the OS base intended to be used by the charm.
	Base corebase.Base

	// InstanceKey is a unique string associated with the application. To
	// assist with keeping KPI data in charmhub, it must be the same for every
	// charmhub Refresh action related to an application. Create with the
	// charmhub.CreateInstanceKey method. LP: 1944582
	InstanceKey string
}

// WithBase allows to update the base of an origin.
func (o Origin) WithBase(b *corebase.Base) Origin {
	other := o
	other.Base = corebase.Base{}
	if b != nil {
		other.Base = *b
	}
	return other
}

// CharmChannel returns the channel indicated by this origin.
func (o Origin) CharmChannel() charm.Channel {
	var track string
	if o.Track != nil {
		track = *o.Track
	}
	var branch string
	if o.Branch != nil {
		branch = *o.Branch
	}
	return charm.Channel{
		Track:  track,
		Risk:   charm.Risk(o.Risk),
		Branch: branch,
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
		Branch:       o.Branch,
		Architecture: o.Architecture,
		Base:         params.Base{Name: o.Base.OS, Channel: o.Base.Channel.String()},
		InstanceKey:  o.InstanceKey,
	}
}

// CoreCharmOrigin is a help method to get a core version of this structure.
func (o Origin) CoreCharmOrigin() corecharm.Origin {
	var track string
	if o.Track != nil {
		track = *o.Track
	}
	var branch string
	if o.Branch != nil {
		branch = *o.Branch
	}
	var channel *charm.Channel
	if o.Risk != "" {
		channel = &charm.Channel{
			Risk:   charm.Risk(o.Risk),
			Track:  track,
			Branch: branch,
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
			OS:           o.Base.OS,
			Channel:      o.Base.Channel.Track,
		},
		InstanceKey: o.InstanceKey,
	}
}

// APICharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func APICharmOrigin(origin params.CharmOrigin) (Origin, error) {
	base, err := corebase.ParseBase(origin.Base.Name, origin.Base.Channel)
	if err != nil {
		return Origin{}, errors.Trace(err)
	}
	return Origin{
		Source:       OriginSource(origin.Source),
		Type:         origin.Type,
		ID:           origin.ID,
		Hash:         origin.Hash,
		Risk:         origin.Risk,
		Revision:     origin.Revision,
		Track:        origin.Track,
		Branch:       origin.Branch,
		Architecture: origin.Architecture,
		Base:         base,
		InstanceKey:  origin.InstanceKey,
	}, nil
}

// CoreCharmOrigin is a helper function to convert params.CharmOrigin
// to an Origin.
func CoreCharmOrigin(origin corecharm.Origin) (Origin, error) {
	var ch charm.Channel
	if origin.Channel != nil {
		ch = *origin.Channel
	}
	var track *string
	if ch.Track != "" {
		track = &ch.Track
	}
	var branch *string
	if ch.Branch != "" {
		branch = &ch.Branch
	}
	chBase, err := corebase.ParseBase(origin.Platform.OS, origin.Platform.Channel)
	if err != nil {
		return Origin{}, errors.Trace(err)
	}
	return Origin{
		Source:       OriginSource(origin.Source),
		Type:         origin.Type,
		ID:           origin.ID,
		Hash:         origin.Hash,
		Revision:     origin.Revision,
		Risk:         string(ch.Risk),
		Track:        track,
		Branch:       branch,
		Architecture: origin.Platform.Architecture,
		Base:         chBase,
		InstanceKey:  origin.InstanceKey,
	}, nil
}
