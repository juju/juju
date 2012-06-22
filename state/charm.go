// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"net/url"
)

// charmData contains the data stored inside the ZooKeeper charm node.
type charmData struct {
	Meta         *charm.Meta
	Config       *charm.Config
	BundleURL    string `yaml:"url"`
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

var _ charm.Charm = (*Charm)(nil)

func newCharm(st *State, curl *charm.URL, data *charmData) (*Charm, error) {
	burl, err := url.Parse(data.BundleURL)
	if err != nil {
		return nil, err
	}
	c := &Charm{
		st:           st,
		url:          curl,
		meta:         data.Meta,
		config:       data.Config,
		bundleURL:    burl,
		bundleSha256: data.BundleSha256,
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

// Charm path returns the full qualified ZooKeeper path for a charm state
// based on the charm URL.
func charmPath(curl *charm.URL) (string, error) {
	if curl.Revision < 0 {
		return "", fmt.Errorf("charm URL revision is unset")
	}
	return fmt.Sprintf("/charms/%s", charm.Quote(curl.String())), nil
}
