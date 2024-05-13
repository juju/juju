// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"io"

	charmresource "github.com/juju/charm/v13/resource"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.resource")

// ResourceData represents the response from store about a request for
// resource bytes.
type ResourceData struct {
	// ReadCloser holds the bytes for the resource.
	io.ReadCloser

	// Resource holds the metadata for the resource.
	Resource charmresource.Resource
}
