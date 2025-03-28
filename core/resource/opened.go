// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"

	"github.com/juju/juju/internal/errors"
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
	return errors.Capture(o.ReadCloser.Close())
}

// Opener exposes the functionality for opening a resource.
type Opener interface {
	// OpenResource returns an opened resource with a reader that will
	// stream the resource content.
	OpenResource(ctx context.Context, name string) (Opened, error)

	// SetResource records that the resource is currently in use.
	SetResourceUsed(ctx context.Context, resName string) error
}
