// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/juju/proxy"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/containermanager"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelconfig"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/errors"
)

// keysForContainerConfig lists the model config keys that we need to determine
// the container config.
var keysForContainerConfig = []string{
	config.EnableOSRefreshUpdateKey,
	config.EnableOSUpgradeKey,
	config.TypeKey,
	config.SSLHostnameVerificationKey,
	config.HTTPProxyKey,
	config.HTTPSProxyKey,
	config.FTPProxyKey,
	config.NoProxyKey,
	config.JujuHTTPProxyKey,
	config.JujuHTTPSProxyKey,
	config.JujuFTPProxyKey,
	config.JujuNoProxyKey,
	config.AptHTTPProxyKey,
	config.AptHTTPSProxyKey,
	config.AptFTPProxyKey,
	config.AptNoProxyKey,
	config.AptMirrorKey,
	config.SnapHTTPProxyKey,
	config.SnapHTTPSProxyKey,
	config.SnapStoreAssertionsKey,
	config.SnapStoreProxyKey,
	config.SnapStoreProxyURLKey,
	config.CloudInitUserDataKey,
	config.ContainerInheritPropertiesKey,
}

// Provider represents an underlying cloud provider.
type Provider interface {
	// SupportsContainerAddresses returns true if the provider is able to
	// allocate addresses for containers. It may return false or an
	// [errors.NotSupported] if container addresses are not supported.
	SupportsContainerAddresses(ctx envcontext.ProviderCallContext) (bool, error)
}

// Service is an agent provisioner service that can be used by the provisioner
// to retrieve container configuration for provisioning.
type Service struct {
	providerGetter func(context.Context) (Provider, error)
	st             State
}

// State provides the service with access to controller/model config.
type State interface {
	// GetModelConfigKeyValues returns a model config object populated with
	// values for the provided keys.
	GetModelConfigKeyValues(context.Context, ...string) (map[string]string, error)
	// ModelID returns the UUID of the current model.
	ModelID(ctx context.Context) (model.UUID, error)
}

// NewService returns a new agent provisioner service.
func NewService(
	st State,
	providerGetter providertracker.ProviderGetter[Provider],
) *Service {
	return &Service{
		providerGetter: providerGetter,
		st:             st,
	}
}

// ContainerManagerConfigForType returns the container manager config for the
// given container type.
func (s *Service) ContainerManagerConfigForType(
	ctx context.Context,
	containerType instance.ContainerType,
) (containermanager.Config, error) {
	rval := containermanager.Config{}

	modelID, err := s.st.ModelID(ctx)
	if err != nil {
		return rval, errors.Errorf("getting ID for current model: %w", err)
	}

	cfg, err := s.st.GetModelConfigKeyValues(ctx,
		config.LXDSnapChannel,
		config.ContainerImageMetadataURLKey,
		config.ContainerImageMetadataDefaultsDisabledKey,
		config.ContainerImageStreamKey,
	)
	if err != nil {
		return rval, errors.Errorf(
			"cannot get model config keys when calculating container manager config: %w",
			err)

	}

	rval.ModelID = modelID
	if containerType == instance.LXD {
		rval.LXDSnapChannel = cfg[config.LXDSnapChannel]
	}
	rval.ImageMetadataURL = cfg[config.ContainerImageMetadataURLKey]
	rval.MetadataDefaultsDisabled, _ = strconv.ParseBool(cfg[config.ContainerImageMetadataDefaultsDisabledKey])
	rval.ImageStream = cfg[config.ContainerImageStreamKey]

	return rval, nil
}

// ContainerNetworkingMethod determines the container networking method that
// should be used, based on the model config key "container-networking-method"
// and the current provider.
func (s *Service) ContainerNetworkingMethod(ctx context.Context) (containermanager.NetworkingMethod, error) {
	cfg, err := s.st.GetModelConfigKeyValues(ctx, config.ContainerNetworkingMethodKey)
	if err != nil {
		return "", errors.Errorf("getting container networking method from model config: %w", err)
	}

	method := modelconfig.ContainerNetworkingMethod(cfg[config.ContainerNetworkingMethodKey])
	switch method {
	case modelconfig.ContainerNetworkingMethodLocal:
		return containermanager.NetworkingMethodLocal, nil
	case modelconfig.ContainerNetworkingMethodProvider:
		return containermanager.NetworkingMethodProvider, nil
	case modelconfig.ContainerNetworkingMethodAuto:
		// Auto-configure container networking method below
	default:
		return "", errors.Errorf("unable to deduce container networking method %q from model config", method)
	}

	provider, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Provider doesn't have the SupportsContainerAddresses method
		return containermanager.NetworkingMethodLocal, nil
	} else if err != nil {
		return "", errors.Errorf(
			"cannot get networking provider for model: %w",
			err)

	}

	supports, err := provider.SupportsContainerAddresses(envcontext.WithoutCredentialInvalidator(ctx))
	if errors.Is(err, coreerrors.NotSupported) {
		return containermanager.NetworkingMethodLocal, nil
	} else if err != nil {
		return "", errors.Errorf(
			"cannot determine if provider supports container addresses: %w",
			err)

	}
	if supports {
		return containermanager.NetworkingMethodProvider, nil
	}
	return containermanager.NetworkingMethodLocal, nil
}

// ContainerConfig returns the container config for the model.
func (s *Service) ContainerConfig(ctx context.Context) (container.Config, error) {
	result := container.Config{}

	modelConfig, err := s.st.GetModelConfigKeyValues(ctx, keysForContainerConfig...)
	if err != nil {
		return result, errors.Errorf(
			"cannot get model config keys when calculating container config: %w",
			err)

	}

	result.EnableOSRefreshUpdate, _ = strconv.ParseBool(modelConfig[config.EnableOSRefreshUpdateKey])
	result.EnableOSUpgrade, _ = strconv.ParseBool(modelConfig[config.EnableOSUpgradeKey])
	result.ProviderType = modelConfig[config.TypeKey]
	result.SSLHostnameVerification, _ = strconv.ParseBool(modelConfig[config.SSLHostnameVerificationKey])
	result.LegacyProxy = proxy.Settings{
		Http:    modelConfig[config.HTTPProxyKey],
		Https:   modelConfig[config.HTTPSProxyKey],
		Ftp:     modelConfig[config.FTPProxyKey],
		NoProxy: modelConfig[config.NoProxyKey],
	}
	result.JujuProxy = proxy.Settings{
		Http:    modelConfig[config.JujuHTTPProxyKey],
		Https:   modelConfig[config.JujuHTTPSProxyKey],
		Ftp:     modelConfig[config.JujuFTPProxyKey],
		NoProxy: modelConfig[config.JujuNoProxyKey],
	}
	result.AptProxy = proxy.Settings{
		Http:    addSchemeIfMissing("http", getWithFallback(modelConfig, config.AptHTTPProxyKey, config.JujuHTTPProxyKey, config.HTTPProxyKey)),
		Https:   addSchemeIfMissing("https", getWithFallback(modelConfig, config.AptHTTPSProxyKey, config.JujuHTTPSProxyKey, config.HTTPSProxyKey)),
		Ftp:     addSchemeIfMissing("ftp", getWithFallback(modelConfig, config.AptFTPProxyKey, config.JujuFTPProxyKey, config.FTPProxyKey)),
		NoProxy: aptNoProxy(modelConfig),
	}
	result.AptMirror = modelConfig[config.AptMirrorKey]
	result.SnapProxy = proxy.Settings{
		Http:  modelConfig[config.SnapHTTPProxyKey],
		Https: modelConfig[config.SnapHTTPSProxyKey],
	}
	result.SnapStoreAssertions = modelConfig[config.SnapStoreAssertionsKey]
	result.SnapStoreProxyID = modelConfig[config.SnapStoreProxyKey]
	result.SnapStoreProxyURL = modelConfig[config.SnapStoreProxyURLKey]
	_ = yaml.Unmarshal([]byte(modelConfig[config.CloudInitUserDataKey]), &result.CloudInitUserData)
	result.ContainerInheritProperties = modelConfig[config.ContainerInheritPropertiesKey]
	return result, nil
}

// TODO: all the following methods were copied from environs/config, to ensure
//  that this service produces exactly the same container config as in Juju 3.
//  The logic here is almost certainly wrong though, and we should rethink the
//  need to do all this. Hopefully bringing this out in the open will allow
//  that to happen.

// addSchemeIfMissing adds a scheme to a URL if it is missing
// Copied from github.com/juju/juju/environs/config
func addSchemeIfMissing(defaultScheme string, url string) string {
	if url != "" && !strings.Contains(url, "://") {
		url = defaultScheme + "://" + url
	}
	return url
}

// Copied from github.com/juju/juju/environs/config
func getWithFallback(c map[string]string, key, fallback1, fallback2 string) string {
	value := c[key]
	if value == "" {
		value = c[fallback1]
	}
	if value == "" {
		value = c[fallback2]
	}
	return value
}

// AptNoProxy returns the 'apt-no-proxy' for the model.
// Copied from github.com/juju/juju/environs/config
func aptNoProxy(c map[string]string) string {
	value := c[config.AptNoProxyKey]
	if value == "" {
		if hasLegacyProxy(c) {
			value = c[config.NoProxyKey]
		} else {
			value = c[config.JujuNoProxyKey]
		}
	}
	return value
}

// HasLegacyProxy returns true if there is any proxy set using the old legacy proxy keys.
// Copied from github.com/juju/juju/environs/config
func hasLegacyProxy(c map[string]string) bool {
	// We exclude the no proxy value as it has default value.
	return c[config.HTTPProxyKey] != "" ||
		c[config.HTTPSProxyKey] != "" ||
		c[config.FTPProxyKey] != ""
}
