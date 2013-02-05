package maas

import (
	"errors"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"strings"
)

var maasConfigChecker = schema.StrictFieldMap(
	schema.Fields{
		"maas-server": schema.String(),
		// maas-oauth is a colon-separated triplet of:
		// consumer-key:resource-token:resource-secret
		"maas-oauth": schema.String(),
	},
	schema.Defaults{},
)

type maasEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *maasEnvironConfig) MAASServer() string {
	value, ok := cfg.attrs["maas-server"]
	if !ok {
		return ""
	}
	return value.(string)
}

func (cfg *maasEnvironConfig) MAASOAuth() string {
	value, ok := cfg.attrs["maas-oauth"]
	if !ok {
		return ""
	}
	return value.(string)
}

func (prov maasEnvironProvider) newConfig(cfg *config.Config) (*maasEnvironConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	result := new(maasEnvironConfig)
	result.Config = validCfg
	result.attrs = validCfg.UnknownAttrs()
	return result, nil
}

var errMalformedMaasOAuth = errors.New("malformed maas-oauth (3 items separated by colons)")

func (prov maasEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	v, err := maasConfigChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	envCfg := new(maasEnvironConfig)
	envCfg.Config = cfg
	envCfg.attrs = v.(map[string]interface{})
	oauth := envCfg.MAASOAuth()
	if strings.Count(oauth, ":") != 2 {
		return nil, errMalformedMaasOAuth
	}
	return cfg.Apply(envCfg.attrs)
}
