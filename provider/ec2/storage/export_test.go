// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/storage"
)

func EBSProvider() storage.Provider {
	return &ebsProvider{}
}
