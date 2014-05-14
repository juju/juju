// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/utils"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
const boilerplateConfig = `joyent:
  type: joyent

  # SDC config
  # Can be set via env variables, or specified here
  # sdc-user: <secret>
  # Can be set via env variables, or specified here
  # sdc-key-id: <secret>
  # url defaults to us-west-1 DC, override if required
  # sdc-url: https://us-west-1.api.joyentcloud.com

  # Manta config
  # Can be set via env variables, or specified here
  # manta-user: <secret>
  # Can be set via env variables, or specified here
  # manta-key-id: <secret>
  # url defaults to us-east DC, override if required
  # manta-url: https://us-east.manta.joyent.com

  # Auth config
  # private-key-path is the private key used to sign Joyent requests.
  # Defaults to ~/.ssh/id_rsa, override if a different ssh key is used.
  # Alternatively, you can supply "private-key" with the content of the private
  # key instead supplying the path to a file.
  # private-key-path: ~/.ssh/id_rsa
  # algorithm defaults to rsa-sha256, override if required
  # algorithm: rsa-sha256
`

const (
	SdcAccount          = "SDC_ACCOUNT"
	SdcKeyId            = "SDC_KEY_ID"
	SdcUrl              = "SDC_URL"
	MantaUser           = "MANTA_USER"
	MantaKeyId          = "MANTA_KEY_ID"
	MantaUrl            = "MANTA_URL"
	MantaPrivateKeyFile = "MANTA_PRIVATE_KEY_FILE"
	DefaultPrivateKey   = "~/.ssh/id_rsa"
)

var environmentVariables = map[string]string{
	"sdc-user":         SdcAccount,
	"sdc-key-id":       SdcKeyId,
	"sdc-url":          SdcUrl,
	"manta-user":       MantaUser,
	"manta-key-id":     MantaKeyId,
	"manta-url":        MantaUrl,
	"private-key-path": MantaPrivateKeyFile,
}

var configFields = schema.Fields{
	"sdc-user":         schema.String(),
	"sdc-key-id":       schema.String(),
	"sdc-url":          schema.String(),
	"manta-user":       schema.String(),
	"manta-key-id":     schema.String(),
	"manta-url":        schema.String(),
	"private-key-path": schema.String(),
	"algorithm":        schema.String(),
	"control-dir":      schema.String(),
	"private-key":      schema.String(),
}

var configDefaults = schema.Defaults{
	"sdc-url":          "https://us-west-1.api.joyentcloud.com",
	"manta-url":        "https://us-east.manta.joyent.com",
	"algorithm":        "rsa-sha256",
	"private-key-path": schema.Omit,
	"sdc-user":         schema.Omit,
	"sdc-key-id":       schema.Omit,
	"manta-user":       schema.Omit,
	"manta-key-id":     schema.Omit,
	"private-key":      schema.Omit,
}

var configSecretFields = []string{
	"sdc-user",
	"sdc-key-id",
	"manta-user",
	"manta-key-id",
	"private-key",
}

var configImmutableFields = []string{
	"sdc-url",
	"manta-url",
	"private-key-path",
	"private-key",
	"algorithm",
}

func prepareConfig(cfg *config.Config) (*config.Config, error) {
	// Turn an incomplete config into a valid one, if possible.
	attrs := cfg.UnknownAttrs()

	if _, ok := attrs["control-dir"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		attrs["control-dir"] = fmt.Sprintf("%x", uuid.Raw())
	}
	return cfg.Apply(attrs)
}

func validateConfig(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}

	newAttrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envConfig := &environConfig{cfg, newAttrs}
	// If an old config was supplied, check any immutable fields have not changed.
	if old != nil {
		oldEnvConfig, err := validateConfig(old, nil)
		if err != nil {
			return nil, err
		}
		for _, field := range configImmutableFields {
			if oldEnvConfig.attrs[field] != envConfig.attrs[field] {
				return nil, fmt.Errorf(
					"%s: cannot change from %v to %v",
					field, oldEnvConfig.attrs[field], envConfig.attrs[field],
				)
			}
		}
	}

	// Read env variables to fill in any missing fields.
	for field, envVar := range environmentVariables {
		// If field is not set, get it from env variables
		if fieldValue, ok := envConfig.attrs[field]; !ok || fieldValue == "" {
			localEnvVariable := os.Getenv(envVar)
			if localEnvVariable != "" {
				envConfig.attrs[field] = localEnvVariable
			} else {
				if field != "private-key-path" {
					return nil, fmt.Errorf("cannot get %s value from environment variable %s", field, envVar)
				}
			}
		}
	}

	// Ensure private-key-path is set - if it's not in config or an env var, use a default value.
	if v, ok := envConfig.attrs["private-key-path"]; !ok || v == "" {
		v = os.Getenv(environmentVariables["private-key-path"])
		if v == "" {
			v = DefaultPrivateKey
		}
		envConfig.attrs["private-key-path"] = v
	}
	// Now that we've ensured private-key-path is properly set, we go back and set
	// up the private key - this is used to sign requests.
	if fieldValue, ok := envConfig.attrs["private-key"]; !ok || fieldValue == "" {
		keyFile, err := utils.NormalizePath(envConfig.attrs["private-key-path"].(string))
		if err != nil {
			return nil, err
		}
		privateKey, err := ioutil.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		envConfig.attrs["private-key"] = string(privateKey)
	}

	// Check for missing fields.
	for field := range configFields {
		if envConfig.attrs[field] == "" {
			return nil, fmt.Errorf("%s: must not be empty", field)
		}
	}
	return envConfig, nil
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (ecfg *environConfig) GetAttrs() map[string]interface{} {
	return ecfg.attrs
}

func (ecfg *environConfig) sdcUrl() string {
	return ecfg.attrs["sdc-url"].(string)
}

func (ecfg *environConfig) sdcUser() string {
	return ecfg.attrs["sdc-user"].(string)
}

func (ecfg *environConfig) sdcKeyId() string {
	return ecfg.attrs["sdc-key-id"].(string)
}

func (ecfg *environConfig) mantaUrl() string {
	return ecfg.attrs["manta-url"].(string)
}

func (ecfg *environConfig) mantaUser() string {
	return ecfg.attrs["manta-user"].(string)
}

func (ecfg *environConfig) mantaKeyId() string {
	return ecfg.attrs["manta-key-id"].(string)
}

func (ecfg *environConfig) privateKey() string {
	if v, ok := ecfg.attrs["private-key"]; ok {
		return v.(string)
	}
	return ""
}

func (ecfg *environConfig) algorithm() string {
	return ecfg.attrs["algorithm"].(string)
}

func (c *environConfig) controlDir() string {
	return c.attrs["control-dir"].(string)
}

func (c *environConfig) ControlDir() string {
	return c.controlDir()
}

func (ecfg *environConfig) SdcUrl() string {
	return ecfg.sdcUrl()
}

func (ecfg *environConfig) Region() string {
	sdcUrl := ecfg.sdcUrl()
	// Check if running against local services
	if isLocalhost(sdcUrl) {
		return "some-region"
	}
	return sdcUrl[strings.LastIndex(sdcUrl, "/")+1 : strings.Index(sdcUrl, ".")]
}

func isLocalhost(u string) bool {
	parsedUrl, err := url.Parse(u)
	if err != nil {
		return false
	}
	if strings.HasPrefix(parsedUrl.Host, "localhost") || strings.HasPrefix(parsedUrl.Host, "127.0.0.") {
		return true
	}

	return false
}
