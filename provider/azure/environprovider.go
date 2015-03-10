// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/gwacl"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

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

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) RestrictedConfigAttributes() []string {
	return []string{"location"}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p azureEnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	// Set availability-sets-enabled to true
	// by default, unless the user set a value.
	if _, ok := cfg.AllAttrs()["availability-sets-enabled"]; !ok {
		var err error
		cfg, err = cfg.Apply(map[string]interface{}{"availability-sets-enabled": true})
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return cfg, nil
}

// PrepareForBootstrap is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	cfg, err := prov.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := prov.Open(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env.(*azureEnviron)); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return env, nil
}

// verifyCredentials issues a cheap, non-modifying request to Azure to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e *azureEnviron) error {
	_, err := e.updateStorageAccountKey(e.getSnapshot())
	switch err := errors.Cause(err).(type) {
	case *gwacl.AzureError:
		if err.Code == "ForbiddenError" {
			logger.Debugf("azure request failed: %v", err)
			return errors.New(`authentication failed

Please ensure the Azure subscription ID and certificate you have specified
are correct. You can obtain your subscription ID from the "Settings" page
in the Azure management console, where you can also upload a new certificate
if necessary.`)
		}
	}
	return err
}
