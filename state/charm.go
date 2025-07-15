// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
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

func (b Base) compatibleWith(other Base) bool {
	if b.OS != other.OS {
		return false
	}
	c1, err := corebase.ParseChannel(b.Channel)
	if err != nil {
		return false
	}
	c2, err := corebase.ParseChannel(other.Channel)
	if err != nil {
		return false
	}
	return c1 == c2
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
	parsedURL, err := charm.ParseURL(curl)
	if err != nil {
		return nil, err
	}
	return st.findCharm(parsedURL)
}

type charmImpl struct {
	charm.Charm
	url string
}

func (c *charmImpl) Meta() *charm.Meta {
	return c.Charm.Meta()
}

func (c *charmImpl) Manifest() *charm.Manifest {
	return c.Charm.Manifest()
}

func (c *charmImpl) Actions() *charm.Actions {
	return c.Charm.Actions()
}

func (c *charmImpl) Config() *charm.Config {
	return c.Charm.Config()
}

func (c *charmImpl) Revision() int {
	return c.Charm.Revision()
}

func (c *charmImpl) URL() string {
	return c.url
}

func (c *charmImpl) Version() string {
	return c.Charm.Version()
}

func fromInternalCharm(ch charm.Charm, url string) CharmRefFull {
	return &charmImpl{
		Charm: ch,
		url:   url,
	}
}

// findCharm returns a charm matching the curl if it exists.  This method should
// be used with deep understanding of charm deployment, refresh, charm revision
// updater. Most code should use Charm above.
// The primary direct use case is AddCharmMetadata. When we asynchronously download
// a charm, the metadata is inserted into the db so that part of deploying or
// refreshing a charm can happen before a charm is actually downloaded. Therefore
// it must be able to find placeholders and update them to allow for a download
// to happen as part of refresh.
func (st *State) findCharm(curl *charm.URL) (CharmRefFull, error) {
	var charmSource applicationcharm.CharmSource
	// We must map the charm schema to the charm source. If the schema is
	// not ch nor local, then it will fail retrieving the charm.
	if curl.Schema == "ch" {
		charmSource = applicationcharm.CharmHubSource
	} else if curl.Schema == "local" {
		charmSource = applicationcharm.LocalSource
	}
	charmService, err := st.charmServiceGetter(coremodel.UUID(st.ModelUUID()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	ch, _, _, err := charmService.GetCharm(context.TODO(), applicationcharm.CharmLocator{
		Name:     curl.Name,
		Revision: curl.Revision,
		Source:   charmSource,
	})
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot retrieve charm %q", curl.Name)
	}
	return fromInternalCharm(ch, curl.String()), nil
}
