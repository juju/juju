// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"net/url"

	"launchpad.net/juju-core/charm"
)

// This module implements a subset of the interface provided by
// state.Charm, as needed by the uniter API.

// TODO: Only the required calls are added as placeholders,
// the actual implementation will come in a follow-up.

// Charm represents the state of a charm in the environment.
type Charm struct {
	st  *State
	url *charm.URL
}

// Strings returns the charm URL as a string.
func (c *Charm) String() string {
	return c.url.String()
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	clone := *c.url
	return &clone
}

// BundleURL returns the url to the charm bundle in
// the provider storage.
func (c *Charm) BundleURL() *url.URL {
	// TODO: Call Uniter.CharmBundleURL()
	panic("not implemented")
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
func (c *Charm) BundleSha256() string {
	// TODO: Call Uniter.CharmBundleSha256()
	panic("not implemented")
}
