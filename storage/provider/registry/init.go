// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"github.com/juju/juju/storage/provider"
)

func init() {
	// Register the providers common to all environments, eg loop, tmpfs etc
	for providerType, p := range provider.CommonProviders() {
		RegisterProvider(providerType, p)
	}
}
