// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"

	"github.com/juju/errors"
)

// Opened provides both the resource info and content.
type Opened struct {
	Resource
	io.ReadCloser
}

// Content returns the "content" for the opened resource.
func (o Opened) Content() Content {
	return Content{
		Data:        o.ReadCloser,
		Size:        o.Size,
		Fingerprint: o.Fingerprint,
	}
}

func (o Opened) Close() error {
	return errors.Trace(o.ReadCloser.Close())
}

// Opener exposes the functionality for opening a resource.
type Opener interface {
	// OpenResource returns an opened resource with a reader that will
	// stream the resource content.
	OpenResource(name string) (Opened, error)
}
