// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

const (
	SdcAccount = "SDC_ACCOUNT"
	SdcKeyId   = "SDC_KEY_ID"
	SdcUrl     = "SDC_URL"

	sdcUser        = "sdc-user"
	sdcKeyId       = "sdc-key-id"
	sdcUrl         = "sdc-url"
	privateKeyPath = "private-key-path"
	algorithm      = "algorithm"
	privateKey     = "private-key"
)

var environmentVariables = map[string]string{
	sdcUser:  SdcAccount,
	sdcKeyId: SdcKeyId,
	sdcUrl:   SdcUrl,
}

var configFields = schema.Fields{
	sdcUser:    schema.String(),
	sdcKeyId:   schema.String(),
	sdcUrl:     schema.String(),
	algorithm:  schema.String(),
	privateKey: schema.String(),
}

var configDefaults = schema.Defaults{
	sdcUrl:     "https://us-west-1.api.joyentcloud.com",
	algorithm:  "rsa-sha256",
	sdcUser:    schema.Omit,
	sdcKeyId:   schema.Omit,
	privateKey: schema.Omit,
}

var requiredFields = []string{
	sdcUrl,
	algorithm,
	sdcUser,
	sdcKeyId,
	// privatekey and privatekeypath are handled separately
}

var configSecretFields = []string{
	sdcUser,
	sdcKeyId,
	privateKey,
}

var configImmutableFields = []string{
	sdcUrl,
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
				return nil, fmt.Errorf("cannot get %s value from environment variable %s", field, envVar)
			}
		}
	}

	if err := ensurePrivateKey(envConfig); err != nil {
		return nil, err
	}

	// Check for missing fields.
	for _, field := range requiredFields {
		if nilOrEmptyString(envConfig.attrs[field]) {
			return nil, fmt.Errorf("%s: must not be empty", field)
		}
	}
	return envConfig, nil
}

// Ensure private-key is set.
func ensurePrivateKey(envConfig *environConfig) error {
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

func (ecfg *environConfig) privateKey() string {
	if v, ok := ecfg.attrs[privateKey]; ok {
		return v.(string)
	}
	return ""
}

func (ecfg *environConfig) algorithm() string {
	return ecfg.attrs[algorithm].(string)
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
