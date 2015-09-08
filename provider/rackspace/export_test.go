// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
)

func NewProvider(innerProvider environs.EnvironProvider) environs.EnvironProvider {
	return environProvider{innerProvider}
}

func NewEnviron(innerEnviron environs.Environ) environs.Environ {
	return environ{innerEnviron}
}
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> More review comments implemented

var Bootstrap = &bootstrap

var NewInstanceConfigurator = &newInstanceConfigurator
<<<<<<< HEAD
=======
>>>>>>> review comments implemented
=======
>>>>>>> More review comments implemented
