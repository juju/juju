// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
)

// Register the Azure provider with Juju.
func init() {
	environs.RegisterProvider("azure", azureEnvironProvider{})
}

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.provider.azure")

type azureEnvironProvider struct{}

// azureEnvironProvider implements EnvironProvider.
var _ environs.EnvironProvider = (*azureEnvironProvider)(nil)

// Open is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening environment %q.", cfg.Name())
	// We can't return NewEnviron(cfg) directly here because otherwise,
	// when err is not nil, we end up with a non-nil returned environ and
	// this breaks the loop in cmd/jujud/upgrade.go:run() (see
	// http://golang.org/doc/faq#nil_error for the gory details).
	environ, err := NewEnviron(cfg)
	if err != nil {
		return nil, err
	}
	return environ, nil
}

// Prepare is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	// Set availability-sets-enabled to true
	// by default, unless the user set a value.
	if _, ok := cfg.AllAttrs()["availability-sets-enabled"]; !ok {
		var err error
		cfg, err = cfg.Apply(map[string]interface{}{"availability-sets-enabled": true})
		if err != nil {
			return nil, err
		}
	}
	return prov.Open(cfg)
}
