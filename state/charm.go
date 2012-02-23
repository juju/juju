// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
)

// charmData contains the data stored inside the ZooKeeper charm node.
type charmData struct {
	Meta   *charm.Meta
	Config *charm.Config
	URL    string
}

// Charm represents the state of a charm in the environment.
type Charm struct {
	zk        *zookeeper.Conn
	url       *charm.URL
	meta      *charm.Meta
	config    *charm.Config
	bundleURL string
}

// readCharm reads a charm by id.
func readCharm(zk *zookeeper.Conn, charmURL *charm.URL) (*Charm, error) {
	yaml, _, err := zk.Get(charmPath(charmURL))
	if err == zookeeper.ZNONODE {
		return nil, fmt.Errorf("charm %q not found", charmURL)
	}
	if err != nil {
		return nil, err
	}
	data := &charmData{}
	if err := goyaml.Unmarshal([]byte(yaml), data); err != nil {
		return nil, err
	}
	return newCharm(zk, charmURL, data)
}

// newCharm creates a new charm.
func newCharm(zk *zookeeper.Conn, charmURL *charm.URL, data *charmData) (*Charm, error) {
	c := &Charm{
		zk:        zk,
		url:       charmURL,
		meta:      data.Meta,
		config:    data.Config,
		bundleURL: data.URL,
	}
	// Just a health check.
	if c.meta.Name != c.Name() {
		return nil, fmt.Errorf("illegal charm name")
	}
	return c, nil
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

// BundleURL returns the url to the charm bundle in 
// the provider storage.
func (c *Charm) BundleURL() string {
	return c.bundleURL
}

// Charm path returns the full qualified ZooKeeper path for a charm state
// based on the URL.
func charmPath(charmURL *charm.URL) string {
	return fmt.Sprintf("/charms/%s", Quote(charmURL.String()))
}
