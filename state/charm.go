// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	jujuversion "github.com/juju/juju/core/version"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
)

// CharmService represents a service for retrieving charms.
type CharmService interface {
	// GetCharm returns the charm by name, source and revision. Calling this method
	// will return all the data associated with the charm. It is not expected to
	// call this method for all calls, instead use the move focused and specific
	// methods. That's because this method is very expensive to call. This is
	// implemented for the cases where all the charm data is needed; model
	// migration, charm export, etc.
	GetCharm(ctx context.Context, locator applicationcharm.CharmLocator) (charm.Charm, applicationcharm.CharmLocator, bool, error)
}

// Channel identifies and describes completely a store channel.
type Channel struct {
	Track  string `bson:"track,omitempty"`
	Risk   string `bson:"risk"`
	Branch string `bson:"branch,omitempty"`
}

// Base identifies the base os the charm was installed on.
type Base struct {
	OS      string `bson:"os"`
	Channel string `bson:"channel"`
}

// Normalise ensures the channel always has a risk.
func (b Base) Normalise() Base {
	if strings.Contains(b.Channel, "/") {
		return b
	}
	nb := b
	nb.Channel = b.Channel + "/stable"
	return nb
}

// DisplayString prints the base without the rask component.
func (b Base) DisplayString() string {
	if b.OS == "" || b.Channel == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", b.OS, strings.Split(b.Channel, "/")[0])
}

func (b Base) String() string {
	if b.OS == "" || b.Channel == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", b.OS, b.Channel)
}

// UbuntuBase is used in tests.
func UbuntuBase(channel string) Base {
	return Base{OS: corebase.UbuntuOS, Channel: channel + "/stable"}
}

// DefaultLTSBase is used in tests.
func DefaultLTSBase() Base {
	return Base{OS: corebase.UbuntuOS, Channel: jujuversion.DefaultSupportedLTSBase().Channel.String()}
}

// Platform identifies the platform the charm was installed on.
type Platform struct {
	Architecture string `bson:"architecture,omitempty"`
	OS           string `bson:"os"`
	Channel      string `bson:"channel"`
}

// CharmOrigin holds the original source of a charm. Information about where the
// charm was installed from (charm-hub, local) and any additional
// information we can utilise when making modelling decisions for upgrading or
// changing.
// Note: InstanceKey should never be added here. See core charm origin definition.
type CharmOrigin struct {
	Source   string    `bson:"source"`
	Type     string    `bson:"type,omitempty"`
	ID       string    `bson:"id"`
	Hash     string    `bson:"hash"`
	Revision *int      `bson:"revision,omitempty"`
	Channel  *Channel  `bson:"channel,omitempty"`
	Platform *Platform `bson:"platform"`
}

// AsCoreCharmOrigin converts a state Origin type into a core/charm.Origin.
func (o CharmOrigin) AsCoreCharmOrigin() corecharm.Origin {
	origin := corecharm.Origin{
		Source:   corecharm.Source(o.Source),
		Type:     o.Type,
		ID:       o.ID,
		Hash:     o.Hash,
		Revision: o.Revision,
	}

	if o.Channel != nil {
		origin.Channel = &charm.Channel{
			Track:  o.Channel.Track,
			Risk:   charm.Risk(o.Channel.Risk),
			Branch: o.Channel.Branch,
		}
	}

	if o.Platform != nil {
		origin.Platform = corecharm.Platform{
			Architecture: o.Platform.Architecture,
			OS:           o.Platform.OS,
			Channel:      o.Platform.Channel,
		}
	}

	return origin
}

// Charm returns the charm with the given URL. Charms pending to be uploaded
// are returned for Charmhub charms. Charm placeholders are never returned.
func (st *State) Charm(curl string) (CharmRefFull, error) {
	return nil, nil
}
