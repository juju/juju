// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/juju/storage/provider/registry"
)

func init() {
	registry.RegisterEnvironStorageProviders("dummy")
}
