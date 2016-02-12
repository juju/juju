// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/juju/resource"
)

// HandleDownload handles a resource download request.
func HandleDownload(name string, deps HandleDownloadDeps) (resource.Opened, error) {
	return deps.OpenResource(name)
}

// HandledDownloadDeps exposes the external dependencies of HandleDownload.
type HandleDownloadDeps interface {
	resource.Opener
}
