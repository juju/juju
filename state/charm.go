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

// Id the identifier of the charm.
func (c *Charm) Id() string {
	return c.url.String()
}

// CharmMock returns a charm for tests as long as
// the logic isn't implemented.
func CharmMock(url string) *Charm {
	u, _ := charm.NewURL(url)
	return &Charm{
		url: u,
	}
}
