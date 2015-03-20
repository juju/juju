// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"github.com/juju/juju/environs"
)

var (
	Provider environs.EnvironProvider = providerInstance
)
