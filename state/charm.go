// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
)

// Charm represents the state of a charm in the environment.
type Charm struct {
	zk  *zookeeper.Conn
	url *charm.URL
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	clone := *c.url
	return &clone
}

// CharmMock returns a charm only for tests! It
// will be removed when the charm implementation
// reached a proper state.
func CharmMock(id string) *Charm {
	url, err := charm.ParseURL(id)
	if err != nil {
		panic(err)
	}
	return &Charm{
		url: url,
	}
}
