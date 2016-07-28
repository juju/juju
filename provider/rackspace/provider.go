// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"strings"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct {
	environs.EnvironProvider
}

var providerInstance *environProvider

// BootstrapConfig is specified in the EnvironProvider interface.
func (p *environProvider) BootstrapConfig(args environs.BootstrapConfigParams) (*config.Config, error) {
	// Rackspace regions are expected to be uppercase, but Juju
	// stores and displays them in lowercase in the CLI. Ensure
	// they're uppercase when they get to the Rackspace API.
	args.Cloud.Region = strings.ToUpper(args.Cloud.Region)
	return p.EnvironProvider.BootstrapConfig(args)
}
