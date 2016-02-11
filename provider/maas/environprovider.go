// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/loggo"
	"github.com/juju/utils"

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
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	attrs["maas-agent-name"] = uuid.String()
	return cfg.Apply(attrs)
}

func (p maasEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, args environs.PrepareForBootstrapParams) (environs.Environ, error) {
	// For MAAS, the endpoint from the cloud definition defines the MAAS server.
	attrs := map[string]interface{}{
		"maas-server": args.CloudEndpoint,
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

	cfg, err = p.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, err
	}
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
	// Verify we can connect to the server and authenticate.
	_, err := env.getMAASClient().GetSubObject("maas").CallGet("get_config", nil)
	if err, ok := err.(gomaasapi.ServerError); ok && err.StatusCode == http.StatusUnauthorized {
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
