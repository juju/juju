// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"fmt"

	corecharm "github.com/juju/juju/core/charm"
)

type errCharmAlreadyStored struct {
	charmURL string
	origin   corecharm.Origin
}

// Error implements error.
func (e errCharmAlreadyStored) Error() string {
	return fmt.Sprintf("charm %q has already been downloaded from origin %v", e.charmURL, e.origin)
}

// NewCharmAlreadyStoredError creates an error that indicates that a charm has
// already been stored using the specified origin.
func NewCharmAlreadyStoredError(charmURL string, origin corecharm.Origin) error {
	return errCharmAlreadyStored{
		charmURL: charmURL,
		origin:   origin,
	}
}
