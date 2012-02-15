// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
)

// charmData contains the data stored inside the ZooKeeper charm node.
type charmData struct {
	Meta   *charm.Meta
	Config *charm.Config
	SHA256 string
	URL    string
}

// Charm represents the state of a charm in the environment.
type Charm struct {
	zk        *zookeeper.Conn
	url       *charm.URL
	meta      *charm.Meta
	config    *charm.Config
	sha256    string
	bundleURL string
}

// newCharm creates a new charm.
func newCharm(zk *zookeeper.Conn, id string, data *charmData) (*Charm, error) {
	url, err := charm.ParseURL(id)
	if err != nil {
		panic(err)
	}
	c := &Charm{
		zk:        zk,
		url:       url,
		meta:      data.Meta,
		config:    data.Config,
		sha256:    data.SHA256,
		bundleURL: data.URL,
	}
	// Just a health check.
	if c.meta.Name != c.Name() {
		// QUESTION: Maybe an error is enough here? In
		// Py it's an assert.
		panic("illegal charm name")
	}
	return c, nil
}

// Id returns the charm id.
func (c *Charm) Id() string {
	return c.url.String()
}

// Name returns the name of the charm based on the URL.
func (c *Charm) Name() string {
	return c.url.Name
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
	clone := *c.meta
	return &clone
}

// Config returns the configuration of the charm.
func (c *Charm) Config() *charm.Config {
	clone := *c.config
	return &clone
}

// SHA256 returns the SHA256 hash of the charm.
func (c *Charm) SHA256() string {
	return c.sha256
}

// BundleURL returns the url to the charm bundle in 
// the provider storage.
func (c *Charm) BundleURL() string {
	return c.bundleURL
}

// Charm path returns the full qualified ZooKeeper path for a charm state
// based on the id.
func charmPath(id string) string {
	return fmt.Sprintf("/charms/%s", Quote(id))
}
