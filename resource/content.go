// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move this file to the charm repo?

import (
	"io"

	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

// Content holds a reader for the content of a resource along
// with details about that content.
type Content struct {
	// Data holds the resouce content, ready to be read (once).
	Data io.Reader

	// Size is the byte count of the data.
	Size int64

	// Fingerprint holds the checksum of the data.
	Fingerprint charmresource.Fingerprint
}
