// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
)

var logger = loggo.GetLogger("juju.provider.rackspace")

type environProvider struct {
	environs.EnvironProvider
}

var providerInstance *environProvider
