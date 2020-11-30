// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"

	"github.com/juju/charm/v8/resource"
)

// Opened provides both the resource info and content.
type Opened struct {
	io.ReadCloser
	Size int64
	Fingerprint resource.Fingerprint
}

// Opener exposes the functionality for opening a resource.
type Opener interface {
	// OpenResource returns an opened resource with a reader that will
	// stream the resource content.
	OpenResource(name string) (Opened, error)
}
