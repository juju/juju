// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
)

type environProvider struct {
	environs.EnvironProvider
}

var providerInstance *environProvider
