// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs/config"
)

const (
	configAttrStorageAccountType = "storage-account-type"

	// The below bits are internal book-keeping things, rather than
	// configuration. Config is just what we have to work with.

	// resourceNameLengthMax is the maximum length of resource
	// names in Azure.
	resourceNameLengthMax = 80
)

var configFields = schema.Fields{
	configAttrStorageAccountType: schema.String(),
}

var configDefaults = schema.Defaults{
	configAttrStorageAccountType: string(storage.StandardLRS),
}

var immutableConfigAttributes = []string{
	configAttrStorageAccountType,
}

type azureModelConfig struct {
	*config.Config
	storageAccountType string
}

var knownStorageAccountTypes = []string{
	"Standard_LRS", "Standard_GRS", "Standard_RAGRS", "Standard_ZRS", "Premium_LRS",
}

// Validate ensures that the provided configuration is valid for this
// provider, and that changes between the old (if provided) and new
// configurations are valid.
func (*azureEnvironProvider) Validate(newCfg, oldCfg *config.Config) (*config.Config, error) {
	_, err := validateConfig(newCfg, oldCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCfg, nil
}

func validateConfig(newCfg, oldCfg *config.Config) (*azureModelConfig, error) {
	err := config.Validate(newCfg, oldCfg)
	if err != nil {
		return nil, err
	}

	validated, err := newCfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}

	if oldCfg != nil {
		// Ensure immutable configuration isn't changed.
		oldUnknownAttrs := oldCfg.UnknownAttrs()
		for _, key := range immutableConfigAttributes {
			oldValue, hadValue := oldUnknownAttrs[key].(string)
			if hadValue {
				newValue, haveValue := validated[key].(string)
				if !haveValue {
					return nil, errors.Errorf(
						"cannot remove immutable %q config", key,
					)
				}
				if newValue != oldValue {
					return nil, errors.Errorf(
						"cannot change immutable %q config (%v -> %v)",
						key, oldValue, newValue,
					)
				}
			}
			// It's valid to go from not having to having.
		}
	}

	// Resource group names must not exceed 80 characters. Resource group
	// names are based on the model UUID and model name, the latter of
	// which the model creator controls.
	modelTag := names.NewModelTag(newCfg.UUID())
	resourceGroup := resourceGroupName(modelTag, newCfg.Name())
	if n := len(resourceGroup); n > resourceNameLengthMax {
		smallestResourceGroup := resourceGroupName(modelTag, "")
		return nil, errors.Errorf(`resource group name %q is too long

Please choose a model name of no more than %d characters.`,
			resourceGroup,
			resourceNameLengthMax-len(smallestResourceGroup),
		)
	}

	if newCfg.FirewallMode() == config.FwGlobal {
		// We do not currently support the "global" firewall mode.
		return nil, errNoFwGlobal
	}

	storageAccountType := validated[configAttrStorageAccountType].(string)
	if !isKnownStorageAccountType(storageAccountType) {
		return nil, errors.Errorf(
			"invalid storage account type %q, expected one of: %q",
			storageAccountType, knownStorageAccountTypes,
		)
	}

	azureConfig := &azureModelConfig{
		newCfg,
		storageAccountType,
	}
	return azureConfig, nil
}

// isKnownStorageAccountType reports whether or not the given string identifies
// a known storage account type.
func isKnownStorageAccountType(t string) bool {
	for _, knownStorageAccountType := range knownStorageAccountTypes {
		if t == knownStorageAccountType {
			return true
		}
	}
	return false
}

// canonicalLocation returns the canonicalized location string. This involves
// stripping whitespace, and lowercasing. The ARM APIs do not support embedded
// whitespace, whereas the old Service Management APIs used to; we allow the
// user to provide either, and canonicalize them to one form that ARM allows.
func canonicalLocation(s string) string {
	s = strings.Replace(s, " ", "", -1)
	return strings.ToLower(s)
}
