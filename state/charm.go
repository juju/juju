// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"

	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	mongoutils "github.com/juju/juju/internal/mongo/utils"
)

// CharmService represents a service for retrieving charms.
type CharmService interface {
	// GetCharm returns the charm using the charm ID.
	// Calling this method will return all the data associated with the charm.
	// It is not expected to call this method for all calls, instead use the move
	// focused and specific methods. That's because this method is very expensive
	// to call. This is implemented for the cases where all the charm data is
	// needed; model migration, charm export, etc.
	GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, applicationcharm.CharmLocator, bool, error)
	// GetCharmID returns a charm ID by name, source and revision. It returns an
	// error if the charm can not be found.
	// This can also be used as a cheap way to see if a charm exists without
	// needing to load the charm metadata.
	GetCharmID(ctx context.Context, args applicationcharm.GetCharmArgs) (corecharm.ID, error)
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

// charmDoc represents the internal state of a charm in MongoDB.
type charmDoc struct {
	ModelUUID string  `bson:"model-uuid"`
	DocID     string  `bson:"_id"`
	URL       *string `bson:"url"`
	// CharmVersion
	CharmVersion string `bson:"charm-version"`

	// Life manages charm lifetime in the usual way, but only local
	// charms can actually be "destroyed"; store charms are
	// immortal.
	Life Life `bson:"life"`

	// These fields are flags; if any of them is set, the charm
	// cannot actually be safely used for anything.
	PendingUpload bool `bson:"pendingupload"`
	Placeholder   bool `bson:"placeholder"`

	// These fields control access to the charm archive.
	BundleSha256 string `bson:"bundlesha256"`
	StoragePath  string `bson:"storagepath"`

	// The remaining fields hold data sufficient to define a
	// charm.Charm.

	// TODO(fwereade) 2015-06-18 lp:1467964
	// DANGEROUS: our schema can change any time the charm package changes,
	// and we have no automated way to detect when that happens. We *must*
	// not depend upon serializations we cannot control from inside this
	// package. What's in a *charm.Meta? What will be tomorrow? What logic
	// will we be writing on the assumption that all stored Metas have set
	// some field? What fields might lose precision when they go into the
	// database?
	Meta       *charm.Meta     `bson:"meta"`
	Config     *charm.Config   `bson:"config"`
	Manifest   *charm.Manifest `bson:"manifest"`
	Actions    *charm.Actions  `bson:"actions"`
	LXDProfile *LXDProfile     `bson:"lxd-profile"`
}

// LXDProfile is the same as ProfilePut defined in github.com/canonical/lxd/shared/api/profile.go
type LXDProfile struct {
	Config      map[string]string            `bson:"config"`
	Description string                       `bson:"description"`
	Devices     map[string]map[string]string `bson:"devices"`
}

// Empty returns true if neither devices nor config have been defined in the profile.
func (profile *LXDProfile) Empty() bool {
	return len(profile.Devices) < 1 && len(profile.Config) < 1
}

// ValidateConfigDevices validates the Config and Devices properties of the LXDProfile.
// WhiteList devices: unix-char, unix-block, gpu, usb.
// BlackList config: boot*, limits* and migration*.
// An empty profile will not return an error.
// TODO (stickupkid): Remove this by moving this up into the API server layer.
func (profile *LXDProfile) ValidateConfigDevices() error {
	for _, val := range profile.Devices {
		goodDevs := set.NewStrings("unix-char", "unix-block", "gpu", "usb")
		if devType, ok := val["type"]; ok {
			if !goodDevs.Contains(devType) {
				return fmt.Errorf("invalid lxd-profile.yaml: contains device type %q", devType)
			}
		}
	}
	for key := range profile.Config {
		if strings.HasPrefix(key, "boot") ||
			strings.HasPrefix(key, "limits") ||
			strings.HasPrefix(key, "migration") {
			return fmt.Errorf("invalid lxd-profile.yaml: contains config value %q", key)
		}
	}
	return nil
}

// CharmInfo contains all the data necessary to store a charm's metadata.
type CharmInfo struct {
	Charm       charm.Charm
	ID          string
	StoragePath string
	SHA256      string
	Version     string
}

// Charm represents the state of a charm in the model.
type Charm struct {
	st       *State
	doc      charmDoc
	charmURL *charm.URL
}

var _ charm.LXDProfiler = (*Charm)(nil)

func newCharm(st *State, cdoc *charmDoc) *Charm {
	// Because we probably just read the doc from state, make sure we
	// unescape any config option names for "$" and ".". See
	// http://pad.lv/1308146
	if cdoc != nil && cdoc.Config != nil {
		unescapedConfig := charm.NewConfig()
		for optionName, option := range cdoc.Config.Options {
			unescapedName := mongoutils.UnescapeKey(optionName)
			unescapedConfig.Options[unescapedName] = option
		}
		cdoc.Config = unescapedConfig
	}

	if cdoc != nil {
		cdoc.LXDProfile = unescapeLXDProfile(cdoc.LXDProfile)
	}

	cdoc.ModelUUID = st.ModelUUID()

	ch := Charm{st: st, doc: *cdoc}
	return &ch
}

// unescapeLXDProfile returns the LXDProfile back to normal after
// reading from state.
func unescapeLXDProfile(profile *LXDProfile) *LXDProfile {
	if profile == nil {
		return nil
	}
	unescapedProfile := &LXDProfile{}
	unescapedProfile.Description = profile.Description
	// we know the size and shape of the type, so let's use UnescapeKey
	unescapedConfig := make(map[string]string, len(profile.Config))
	for k, v := range profile.Config {
		unescapedConfig[mongoutils.UnescapeKey(k)] = v
	}
	unescapedProfile.Config = unescapedConfig
	// this is more easy to reason about than using mongoutils.UnescapeKeys, which
	// requires looping from map[string]interface{} -> map[string]map[string]string
	unescapedDevices := make(map[string]map[string]string, len(profile.Devices))
	for k, v := range profile.Devices {
		nested := make(map[string]string, len(v))
		for vk, vv := range v {
			nested[mongoutils.UnescapeKey(vk)] = vv
		}
		unescapedDevices[mongoutils.UnescapeKey(k)] = nested
	}
	unescapedProfile.Devices = unescapedDevices
	return unescapedProfile
}

// Tag returns a tag identifying the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) Tag() names.Tag {
	return names.NewCharmTag(c.URL())
}

// Life returns the charm's life state.
func (c *Charm) Life() Life {
	return c.doc.Life
}

// Refresh loads fresh charm data from the database. In practice, the
// only observable change should be to its Life value.
func (c *Charm) Refresh() error {
	_, err := c.st.Charm(c.URL())
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(nvinuesa): We have broken refresh, until we fully implement
	// the new charm service, this method is a no-op.
	// See https://warthogs.atlassian.net/browse/JUJU-4767
	// c.doc = ch.doc
	return nil
}

// charmGlobalKey returns the global database key for the charm
// with the given url.
func charmGlobalKey(charmURL *string) string {
	return "c#" + *charmURL
}

// GlobalKey returns the global database key for the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) globalKey() string {
	return charmGlobalKey(c.doc.URL)
}

// URL returns a string which identifies the charm
// The string will parse into a charm.URL if required
func (c *Charm) URL() string {
	if c.doc.URL == nil {
		return ""
	}
	return *c.doc.URL
}

// Revision returns the monotonically increasing charm
// revision number.
// Parse the charm's URL on demand, if required.
func (c *Charm) Revision() int {
	if c.charmURL == nil {
		c.charmURL = charm.MustParseURL(c.URL())
	}
	return c.charmURL.Revision
}

// Version returns the charm version.
func (c *Charm) Version() string {
	return c.doc.CharmVersion
}

// Meta returns the metadata of the charm.
func (c *Charm) Meta() *charm.Meta {
	return c.doc.Meta
}

// Config returns the configuration of the charm.
func (c *Charm) Config() *charm.Config {
	return c.doc.Config
}

// Manifest returns information resulting from the charm
// build process such as the bases on which it can run.
func (c *Charm) Manifest() *charm.Manifest {
	return c.doc.Manifest
}

// Actions returns the actions definition of the charm.
func (c *Charm) Actions() *charm.Actions {
	return c.doc.Actions
}

// LXDProfile returns the lxd profile definition of the charm.
func (c *Charm) LXDProfile() *charm.LXDProfile {
	if c.doc.LXDProfile == nil {
		return nil
	}
	return &charm.LXDProfile{
		Config:      c.doc.LXDProfile.Config,
		Description: c.doc.LXDProfile.Description,
		Devices:     c.doc.LXDProfile.Devices,
	}
}

// StoragePath returns the storage path of the charm bundle.
func (c *Charm) StoragePath() string {
	return c.doc.StoragePath
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
func (c *Charm) BundleSha256() string {
	return c.doc.BundleSha256
}

// IsUploaded returns whether the charm has been uploaded to the
// model storage. Response is only valid when the charm is
// not a placeholder.
// TODO - hml - 29-Mar-2023
// Find a better answer than not pendingupload. Currently
// this method returns true for placeholder charm docs as well,
// thus the result cannot be evaluated independently.
func (c *Charm) IsUploaded() bool {
	return !c.doc.PendingUpload
}

// IsPlaceholder returns whether the charm record is just a placeholder
// rather than representing a deployed charm.
func (c *Charm) IsPlaceholder() bool {
	return c.doc.Placeholder
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
	charmService := st.charmServiceGetter(coremodel.UUID(st.ModelUUID()))
	charmID, err := charmService.GetCharmID(context.TODO(), applicationcharm.GetCharmArgs{
		Name:     curl.Name,
		Revision: &curl.Revision,
		Source:   charmSource,
	})
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get charm ID for URL %q", curl)
	}
	ch, _, _, err := charmService.GetCharm(context.TODO(), charmID)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot retrieve charm with ID %q", charmID.String())
	}
	return fromInternalCharm(ch, curl.String()), nil
}

// LatestPlaceholderCharm returns the latest charm described by the
// given URL but which is not yet deployed.
func (st *State) LatestPlaceholderCharm(curl *charm.URL) (*Charm, error) {
	charms, closer := st.db().GetCollection(charmsC)
	defer closer()

	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(st.docID(noRevURL.String()))
	var docs []charmDoc
	err := charms.Find(bson.D{{"_id", bson.D{{"$regex", curlRegex}}}, {"placeholder", true}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get charm %q", curl)
	}
	// Find the highest revision.
	var latest charmDoc
	var latestURL *charm.URL
	for _, doc := range docs {
		if latest.URL == nil {
			latest = doc
			latestURL, err = charm.ParseURL(*latest.URL)
			if err != nil {
				return nil, errors.Annotatef(err, "latest charm url")
			}
		}
		docURL, err := charm.ParseURL(*doc.URL)
		if err != nil {
			return nil, errors.Annotatef(err, "current charm url")
		}
		if docURL.Revision > latestURL.Revision {
			latest = doc
			latestURL, err = charm.ParseURL(*latest.URL)
			if err != nil {
				return nil, errors.Annotatef(err, "latest charm url")
			}
		}
	}
	if latest.URL == nil {
		return nil, errors.NotFoundf("placeholder charm %q", noRevURL)
	}
	return newCharm(st, &latest), nil
}

const charmRevSeqPrefix = "charmrev-"

func isCharmRevSeqName(name string) bool {
	return strings.HasPrefix(name, charmRevSeqPrefix)
}
