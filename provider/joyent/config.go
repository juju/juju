// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/config"
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
  # Alternatively, you can supply "private-key" with the content of the private
  # key instead supplying the path to a file.
  # private-key-path: ~/.ssh/foo_id
  # algorithm defaults to rsa-sha256, override if required
  # algorithm: rsa-sha256

  # Whether or not to refresh the list of available updates for an
  # OS. The default option of true is recommended for use in
  # production systems, but disabling this can speed up local
  # deployments for development or testing.
  #
  # enable-os-refresh-update: true

  # Whether or not to perform OS upgrades when machines are
  # provisioned. The default option of true is recommended for use
  # in production systems, but disabling this can speed up local
  # deployments for development or testing.
  #
  # enable-os-upgrade: true

`

const (
	SdcAccount          = "SDC_ACCOUNT"
	SdcKeyId            = "SDC_KEY_ID"
	SdcUrl              = "SDC_URL"
	MantaUser           = "MANTA_USER"
	MantaKeyId          = "MANTA_KEY_ID"
	MantaUrl            = "MANTA_URL"
	MantaPrivateKeyFile = "MANTA_PRIVATE_KEY_FILE"

	sdcUser        = "sdc-user"
	sdcKeyId       = "sdc-key-id"
	sdcUrl         = "sdc-url"
	mantaUser      = "manta-user"
	mantaKeyId     = "manta-key-id"
	mantaUrl       = "manta-url"
	privateKeyPath = "private-key-path"
	algorithm      = "algorithm"
	controlDir     = "control-dir"
	privateKey     = "private-key"
)

var environmentVariables = map[string]string{
	sdcUser:        SdcAccount,
	sdcKeyId:       SdcKeyId,
	sdcUrl:         SdcUrl,
	mantaUser:      MantaUser,
	mantaKeyId:     MantaKeyId,
	mantaUrl:       MantaUrl,
	privateKeyPath: MantaPrivateKeyFile,
}

var configFields = schema.Fields{
	sdcUser:        schema.String(),
	sdcKeyId:       schema.String(),
	sdcUrl:         schema.String(),
	mantaUser:      schema.String(),
	mantaKeyId:     schema.String(),
	mantaUrl:       schema.String(),
	privateKeyPath: schema.String(),
	algorithm:      schema.String(),
	controlDir:     schema.String(),
	privateKey:     schema.String(),
}

var configDefaults = schema.Defaults{
	sdcUrl:         "https://us-west-1.api.joyentcloud.com",
	mantaUrl:       "https://us-east.manta.joyent.com",
	algorithm:      "rsa-sha256",
	privateKeyPath: schema.Omit,
	sdcUser:        schema.Omit,
	sdcKeyId:       schema.Omit,
	mantaUser:      schema.Omit,
	mantaKeyId:     schema.Omit,
	privateKey:     schema.Omit,
}

var requiredFields = []string{
	sdcUrl,
	mantaUrl,
	algorithm,
	sdcUser,
	sdcKeyId,
	mantaUser,
	mantaKeyId,
	// privatekey and privatekeypath are handled separately
}

var configSecretFields = []string{
	sdcUser,
	sdcKeyId,
	mantaUser,
	mantaKeyId,
	privateKey,
}

var configImmutableFields = []string{
	sdcUrl,
	mantaUrl,
	privateKeyPath,
	privateKey,
	algorithm,
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
		if nilOrEmptyString(envConfig.attrs[field]) {
			localEnvVariable := os.Getenv(envVar)
			if localEnvVariable != "" {
				envConfig.attrs[field] = localEnvVariable
			} else {
				if field != privateKeyPath {
					return nil, fmt.Errorf("cannot get %s value from environment variable %s", field, envVar)
				}
			}
		}
	}

	if err := ensurePrivateKeyOrPath(envConfig); err != nil {
		return nil, err
	}

	// Now that we've ensured private-key-path is properly set, we go back and set
	// up the private key - this is used to sign requests.
	if nilOrEmptyString(envConfig.attrs[privateKey]) {
		keyFile, err := utils.NormalizePath(envConfig.attrs[privateKeyPath].(string))
		if err != nil {
			return nil, err
		}
		priv, err := ioutil.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		envConfig.attrs[privateKey] = string(priv)
	}

	// Check for missing fields.
	for _, field := range requiredFields {
		if nilOrEmptyString(envConfig.attrs[field]) {
			return nil, fmt.Errorf("%s: must not be empty", field)
		}
	}
	return envConfig, nil
}

// Ensure private-key-path is set.
func ensurePrivateKeyOrPath(envConfig *environConfig) error {
	if !nilOrEmptyString(envConfig.attrs[privateKeyPath]) {
		return nil
	}
	if path := os.Getenv(environmentVariables[privateKeyPath]); path != "" {
		envConfig.attrs[privateKeyPath] = path
		return nil
	}
	if !nilOrEmptyString(envConfig.attrs[privateKey]) {
		return nil
	}

	return errors.New("no ssh private key specified in joyent configuration")
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (ecfg *environConfig) GetAttrs() map[string]interface{} {
	return ecfg.attrs
}

func (ecfg *environConfig) sdcUrl() string {
	return ecfg.attrs[sdcUrl].(string)
}

func (ecfg *environConfig) sdcUser() string {
	return ecfg.attrs[sdcUser].(string)
}

func (ecfg *environConfig) sdcKeyId() string {
	return ecfg.attrs[sdcKeyId].(string)
}

func (ecfg *environConfig) mantaUrl() string {
	return ecfg.attrs[mantaUrl].(string)
}

func (ecfg *environConfig) mantaUser() string {
	return ecfg.attrs[mantaUser].(string)
}

func (ecfg *environConfig) mantaKeyId() string {
	return ecfg.attrs[mantaKeyId].(string)
}

func (ecfg *environConfig) privateKey() string {
	if v, ok := ecfg.attrs[privateKey]; ok {
		return v.(string)
	}
	return ""
}

func (ecfg *environConfig) algorithm() string {
	return ecfg.attrs[algorithm].(string)
}

func (c *environConfig) controlDir() string {
	return c.attrs[controlDir].(string)
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

func nilOrEmptyString(i interface{}) bool {
	return i == nil || i == ""
}
