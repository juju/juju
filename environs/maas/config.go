package maas

import (
	"errors"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var maasConfigChecker = schema.StrictFieldMap(
	schema.Fields{
		"maas-server":  schema.String(),
		"maas-oauth":   schema.List(schema.String()),
		"admin-secret": schema.String(),
	},
	schema.Defaults{
		"maas-server":  "",
		"admin-secret": "",
	},
)

type maasEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *maasEnvironConfig) maasServer() string {
	return cfg.attrs["maas-server"].(string)
}

func (cfg *maasEnvironConfig) maasOAuth() []string {
	return cfg.attrs["maas-oauth"].([]string)
}

func (cfg *maasEnvironConfig) adminSecret() string {
	return cfg.attrs["admin-secret"].(string)
}

func (prov maasEnvironProvider) newConfig(cfg *config.Config) (*maasEnvironConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &maasEnvironConfig{validCfg, validCfg.UnknownAttrs()}, nil
}

var noMaasServer = errors.New("No maas-server configured.")
var noMaasOAuth = errors.New("No maas-oauth configured.")
var malformedMaasOAuth = errors.New("Malformed maas-oauth (expected a list of 3 items).")
var noAdminSecret = errors.New("No admin-secret configured.")

func (prov maasEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	v, err := maasConfigChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	envCfg := &maasEnvironConfig{cfg, v.(map[string]interface{})}
	if envCfg.maasServer() == "" {
		return nil, noMaasServer
	}
	oauth := envCfg.maasOAuth()
	if len(oauth) == 0 {
		return nil, noMaasOAuth
	}
	if len(oauth) != 3 {
		return nil, malformedMaasOAuth
	}
	if envCfg.adminSecret() == "" {
		return nil, noAdminSecret
	}
	return cfg.Apply(envCfg.attrs)
}
