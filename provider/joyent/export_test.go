// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"launchpad.net/juju-core/environs"
	jp "launchpad.net/juju-core/provider/joyent"
)

var Provider environs.EnvironProvider = jp.GetProviderInstance()
