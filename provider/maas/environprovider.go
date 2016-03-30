// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// Logger for the MAAS provider.
var logger = loggo.GetLogger("juju.provider.maas")

type maasEnvironProvider struct {
	environProviderCredentials
}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

var providerInstance maasEnvironProvider

func (maasEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening model %q.", cfg.Name())
	env, err := NewEnviron(cfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}

var errAgentNameAlreadySet = errors.New(
	"maas-agent-name is already set; this should not be set by hand")

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p maasEnvironProvider) RestrictedConfigAttributes() []string {
	return []string{"maas-server"}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p maasEnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	attrs := cfg.UnknownAttrs()
	oldName, found := attrs["maas-agent-name"]
	if found && oldName != "" {
		return nil, errAgentNameAlreadySet
	}
	attrs["maas-agent-name"] = cfg.UUID()
	return cfg.Apply(attrs)
}

// BootstrapConfig is specified in the EnvironProvider interface.
func (p maasEnvironProvider) BootstrapConfig(args environs.BootstrapConfigParams) (*config.Config, error) {
	// For MAAS, either:
	// 1. the endpoint from the cloud definition defines the MAAS server URL
	//    (if a full cloud definition had been set up)
	// 2. the region defines the MAAS server ip/host
	//    (if the bootstrap shortcut is used)
	server := args.CloudEndpoint
	if server == "" && args.CloudRegion != "" {
		server = fmt.Sprintf("http://%s/MAAS", args.CloudRegion)
	}
	if server == "" {
		return nil, errors.New("MAAS server not specified")
	}
	attrs := map[string]interface{}{
		"maas-server": server,
	}
	// Add the credentials.
	switch authType := args.Credentials.AuthType(); authType {
	case cloud.OAuth1AuthType:
		credentialAttrs := args.Credentials.Attributes()
		for k, v := range credentialAttrs {
			attrs[k] = v
		}
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}
	cfg, err := args.Config.Apply(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.PrepareForCreateEnvironment(cfg)
}

// PrepareForBootstrap is specified in the EnvironProvider interface.
func (p maasEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	env, err := p.Open(cfg)
	if err != nil {
		return nil, err
	}
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env.(*maasEnviron)); err != nil {
			return nil, err
		}
	}
	return env, nil
}

func verifyCredentials(env *maasEnviron) error {
	var err error
	// Verify we can connect to the server and authenticate.
	// TODO (mfoord): horrible hardcoded version check.
	if env.apiVersion == "2.0" {
		// TODO (mfoord): use a lighterweight endpoint than machines.
		// Could implement /api/2.0/maas/ op=get_config in new API
		// layer.
		_, err = env.maasController.Machines(gomaasapi.MachinesParams{})
	} else {
		_, err = env.getMAASClient().GetSubObject("maas").CallGet("get_config", nil)
	}
	if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusUnauthorized {
		logger.Debugf("authentication failed: %v", err)
		return errors.New(`authentication failed.

Please ensure the credentials are correct.`)
	}
	return nil
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	secretAttrs := make(map[string]string)
	maasCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["maas-oauth"] = maasCfg.maasOAuth()
	return secretAttrs, nil
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p maasEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}
