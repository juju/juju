package mstate

import (
	"launchpad.net/juju-core/charm"
	"net/url"
)

// charmDoc represents the internal state of a charm in MongoDB.
type charmDoc struct {
	Url          *charm.URL `bson:"_id"`
	Meta         *charm.Meta
	Config       *charm.Config
	BundleURL    string
	BundleSha256 string
}

// Charm represents the state of a charm in the environment.
type Charm struct {
	st           *State
	url          *charm.URL
	meta         *charm.Meta
	config       *charm.Config
	bundleURL    *url.URL
	bundleSha256 string
}

func newCharm(st *State, cdoc *charmDoc) (*Charm, error) {
	burl, err := url.Parse(cdoc.BundleURL)
	if err != nil {
		return nil, err
	}
	c := &Charm{
		st:           st,
		url:          cdoc.Url,
		meta:         cdoc.Meta,
		config:       cdoc.Config,
		bundleURL:    burl,
		bundleSha256: cdoc.BundleSha256,
	}
	return c, nil
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	clone := *c.url
	return &clone
}

// Revision returns the monotonically increasing charm 
// revision number.
func (c *Charm) Revision() int {
	return c.url.Revision
}

// Meta returns the metadata of the charm.
func (c *Charm) Meta() *charm.Meta {
	return c.meta
}

// Config returns the configuration of the charm.
func (c *Charm) Config() *charm.Config {
	return c.config
}

// BundleURL returns the url to the charm bundle in 
// the provider storage.
func (c *Charm) BundleURL() *url.URL {
	return c.bundleURL
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
func (c *Charm) BundleSha256() string {
	return c.bundleSha256
}
