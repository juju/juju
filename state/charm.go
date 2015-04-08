// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net/url"

	"github.com/juju/names"
	"gopkg.in/juju/charm.v5-unstable"
)

// charmDoc represents the internal state of a charm in MongoDB.
type charmDoc struct {
	DocID   string     `bson:"_id"`
	URL     *charm.URL `bson:"url"`
	EnvUUID string     `bson:"env-uuid"`
	Meta    *charm.Meta
	Config  *charm.Config
	Actions *charm.Actions
	Metrics *charm.Metrics

	// DEPRECATED: BundleURL is deprecated, and exists here
	// only for migration purposes. We should remove this
	// when migrations are no longer necessary.
	BundleURL *url.URL `bson:"bundleurl,omitempty"`

	BundleSha256  string
	StoragePath   string
	PendingUpload bool
	Placeholder   bool
}

// Charm represents the state of a charm in the environment.
type Charm struct {
	st  *State
	doc charmDoc
}

func newCharm(st *State, cdoc *charmDoc) *Charm {
	// Because we probably just read the doc from state, make sure we
	// unescape any config option names for "$" and ".". See
	// http://pad.lv/1308146
	if cdoc != nil && cdoc.Config != nil {
		unescapedConfig := charm.NewConfig()
		for optionName, option := range cdoc.Config.Options {
			unescapedName := unescapeReplacer.Replace(optionName)
			unescapedConfig.Options[unescapedName] = option
		}
		cdoc.Config = unescapedConfig
	}
	return &Charm{st: st, doc: *cdoc}
}

// Tag returns a tag identifying the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) Tag() names.Tag {
	return names.NewCharmTag(c.URL().String())
}

// charmGlobalKey returns the global database key for the charm
// with the given url.
func charmGlobalKey(charmURL *charm.URL) string {
	return "c#" + charmURL.String()
}

// GlobalKey returns the global database key for the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) globalKey() string {
	return charmGlobalKey(c.doc.URL)
}

func (c *Charm) String() string {
	return c.doc.URL.String()
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	clone := *c.doc.URL
	return &clone
}

// Revision returns the monotonically increasing charm
// revision number.
func (c *Charm) Revision() int {
	return c.doc.URL.Revision
}

// Meta returns the metadata of the charm.
func (c *Charm) Meta() *charm.Meta {
	return c.doc.Meta
}

// Config returns the configuration of the charm.
func (c *Charm) Config() *charm.Config {
	return c.doc.Config
}

// Metrics returns the metrics declared for the charm.
func (c *Charm) Metrics() *charm.Metrics {
	return c.doc.Metrics
}

// Actions returns the actions definition of the charm.
func (c *Charm) Actions() *charm.Actions {
	return c.doc.Actions
}

// StoragePath returns the storage path of the charm bundle.
func (c *Charm) StoragePath() string {
	return c.doc.StoragePath
}

// BundleURL returns the url to the charm bundle in
// the provider storage.
//
// DEPRECATED: this is only to be used for migrating
// charm archives to environment storage.
func (c *Charm) BundleURL() *url.URL {
	return c.doc.BundleURL
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
func (c *Charm) BundleSha256() string {
	return c.doc.BundleSha256
}

// IsUploaded returns whether the charm has been uploaded to the
// environment storage.
func (c *Charm) IsUploaded() bool {
	return !c.doc.PendingUpload
}

// IsPlaceholder returns whether the charm record is just a placeholder
// rather than representing a deployed charm.
func (c *Charm) IsPlaceholder() bool {
	return c.doc.Placeholder
}
