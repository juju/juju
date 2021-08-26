// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"fmt"
)

type errCharmAlreadyStored struct {
	charmURL string
}

// Error implements error.
func (e errCharmAlreadyStored) Error() string {
	return fmt.Sprintf("charm %q has already been downloaded", e.charmURL)
}

// NewCharmAlreadyStoredError creates an error that indicates that a charm has
// already been stored.
func NewCharmAlreadyStoredError(charmURL string) error {
	return errCharmAlreadyStored{
		charmURL: charmURL,
	}
}
