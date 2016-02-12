// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move this file to the charm repo?

import (
	"io"
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
