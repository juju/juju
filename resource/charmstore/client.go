// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"

	"gopkg.in/juju/charm.v6-unstable"
)

// Client exposes the functionality of a charm store client as needed
// for charm store operations for Juju resources.
type Client interface {
	io.Closer

	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store.
	GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error)
}
