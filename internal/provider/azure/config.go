// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
)

const (
	// configAttrLoadBalancerSkuName mirrors the LoadBalancerSkuName type in the Azure SDK
	configAttrLoadBalancerSkuName = "load-balancer-sku-name"

	// configAttrResourceGroupName specifies an existing resource group to use
	// rather than Juju creating it.
	configAttrResourceGroupName = "resource-group-name"

	// configNetwork is the virtual network each machine's primary NIC
	// is attached to.
	configAttrNetwork = "network"

	// The below bits are internal book-keeping things, rather than
	// configuration. Config is just what we have to work with.

	// resourceNameLengthMax is the maximum length of resource
	// names in Azure.
	resourceNameLengthMax = 80
)

var configSchema = configschema.Fields{
	configAttrLoadBalancerSkuName: {
		Description: "mirrors the LoadBalancerSkuName type in the Azure SDK",
		Type:        configschema.Tstring,
		Mandatory:   true,
	},
	configAttrResourceGroupName: {
		Description: "If set, use the specified resource group for all model artefacts instead of creating one based on the model UUID.",
		Type:        configschema.Tstring,
		Immutable:   true,
	},
	configAttrNetwork: {
		Description: "If set, use the specified virtual network for all model machines instead of creating one.",
		Type:        configschema.Tstring,
		Immutable:   true,
	},
}

var configDefaults = schema.Defaults{
	configAttrLoadBalancerSkuName: string(armnetwork.LoadBalancerSKUNameStandard),
	configAttrResourceGroupName:   schema.Omit,
	configAttrNetwork:             schema.Omit,
}

// Schema returns the configuration schema for an environment.
func (azureEnvironProvider) Schema() configschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p azureEnvironProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p azureEnvironProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (prov *azureEnvironProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.StorageDefaultBlockSourceKey: azureStorageProviderType,
	}, nil
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

type azureModelConfig struct {
	*config.Config

	// Azure specific config.
	loadBalancerSkuName string
	resourceGroupName   string
	virtualNetworkName  string
}

// knownLoadBalancerSkuNames returns a list of valid load balancer SKU names.
func knownLoadBalancerSkuNames() (skus []string) {
	for _, name := range armnetwork.PossibleLoadBalancerSKUNameValues() {
		skus = append(skus, string(name))
	}
	return skus
}

// Validate ensures that the provided configuration is valid for this
// provider, and that changes between the old (if provided) and new
// configurations are valid.
func (*azureEnvironProvider) Validate(ctx context.Context, newCfg, oldCfg *config.Config) (*config.Config, error) {
	_, err := validateConfig(ctx, newCfg, oldCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCfg, nil
}

func validateConfig(ctx context.Context, newCfg, oldCfg *config.Config) (*azureModelConfig, error) {
	err := config.Validate(ctx, newCfg, oldCfg)
	if err != nil {
		return nil, err
	}

	validated, err := newCfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}

	for key, field := range configSchema {
		newValue, haveValue := validated[key]
		if (!haveValue || fmt.Sprintf("%v", newValue) == "") && field.Mandatory {
			return nil, errors.NotValidf("empty value for %q", key)
		}
		if oldCfg == nil {
			continue
		}
		if !field.Immutable {
			continue
		}
		// Ensure immutable configuration isn't changed.
		oldUnknownAttrs := oldCfg.UnknownAttrs()
		oldValue, hadValue := oldUnknownAttrs[key]
		if hadValue {
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

	// Resource group names must not exceed 80 characters. Resource group
	// names are based on the model UUID and model name, the latter of
	// which the model creator controls.
	var userSpecifiedResourceGroup string
	resourceGroup, ok := validated[configAttrResourceGroupName].(string)
	if ok && resourceGroup != "" {
		userSpecifiedResourceGroup = resourceGroup
		if len(resourceGroup) > resourceNameLengthMax {
			return nil, errors.Errorf(`resource group name %q is too long

Please choose a name of no more than %d characters.`,
				resourceGroup,
				resourceNameLengthMax,
			)
		}
	} else {
		modelTag := names.NewModelTag(newCfg.UUID())
		resourceGroup = resourceGroupName(modelTag, newCfg.Name())
		if n := len(resourceGroup); n > resourceNameLengthMax {
			smallestResourceGroup := resourceGroupName(modelTag, "")
			return nil, errors.Errorf(`resource group name %q is too long

Please choose a model name of no more than %d characters.`,
				resourceGroup,
				resourceNameLengthMax-len(smallestResourceGroup),
			)
		}
	}

	if newCfg.FirewallMode() == config.FwGlobal {
		return nil, errors.New("global firewall mode is not supported")
	}

	loadBalancerSkuName, ok := validated[configAttrLoadBalancerSkuName].(string)
	if ok {
		loadBalancerSkuNameTitle := strings.Title(loadBalancerSkuName)
		if loadBalancerSkuName != loadBalancerSkuNameTitle {
			loadBalancerSkuName = loadBalancerSkuNameTitle
			logger.Infof(ctx, "using %q for config parameter %s", loadBalancerSkuName, configAttrLoadBalancerSkuName)
		}
		if !isKnownLoadBalancerSkuName(loadBalancerSkuName) {
			return nil, errors.Errorf(
				"invalid load balancer SKU name %q, expected one of: %q",
				loadBalancerSkuName, knownLoadBalancerSkuNames(),
			)
		}
	} else {
		loadBalancerSkuName = string(armnetwork.LoadBalancerSKUNameStandard)
	}

	networkName, _ := validated[configAttrNetwork].(string)

	azureConfig := &azureModelConfig{
		Config:              newCfg,
		loadBalancerSkuName: loadBalancerSkuName,
		resourceGroupName:   userSpecifiedResourceGroup,
		virtualNetworkName:  networkName,
	}
	return azureConfig, nil
}

// isKnownLoadBalancerSkuName reports whether or not the given string
// a valid storage SKU within the Azure SDK
func isKnownLoadBalancerSkuName(n string) bool {
	for _, skuName := range knownLoadBalancerSkuNames() {
		if n == skuName {
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
