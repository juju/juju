// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/rpc/params"
)

type Provider interface {
	environs.Networking
}

type Service struct {
	modelID        model.UUID
	providerGetter func(context.Context) (Provider, error)
	st             State
}

type State interface {
	// GetModelConfigKeyValues returns a model config object populated with
	// values for the provided keys.
	GetModelConfigKeyValues(context.Context, []string) (map[string]string, error)
	// GetControllerConfigKeyValues returns a controller config object
	// populated with values for the provided keys.
	GetControllerConfigKeyValues(context.Context, []string) (map[string]string, error)
}

func NewService(
	modelID model.UUID,
	st State,
	providerGetter providertracker.ProviderGetter[Provider],
) *Service {
	return &Service{
		providerGetter: providerGetter,
		st:             st,
	}
}

func (s *Service) ContainerManagerConfigForType(
	ctx context.Context,
	containerType instance.ContainerType,
) (containermanager.Config, error) {
	rval := containermanager.Config{}

	cfg, err := s.st.GetModelConfigKeyValues(ctx, []string{
		config.LXDSnapChannel,
		config.ContainerImageMetadataURLKey,
		config.ContainerImageMetadataDefaultsDisabledKey,
		config.ContainerImageStreamKey,
	})
	if err != nil {
		return containermanager.Config{}, fmt.Errorf(
			"cannot get model config keys when calculating container manager config: %w",
			err,
		)
	}

	if containerType == instance.LXD {
		rval.LXDSnapChannel = cfg[config.LXDSnapChannel]
	}

	rval.ImageMetadataURL = cfg[config.ContainerImageMetadataURLKey]
	rval.MetadataDefaultsDisabled, _ = strconv.ParseBool(cfg[config.ContainerImageMetadataDefaultsDisabledKey])
	rval.ImageStream = cfg[config.ContainerImageStreamKey]

	provider, err := s.providerGetter(ctx)
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return containermanager.Config{}, fmt.Errorf(
			"cannot get networking provider for model when calculating container manager config: %w",
			err,
		)
	}

	rval.NetworkingMethod = containermanager.NetworkingMethodLocal
	if provider != nil {
		supports, err := provider.SupportsContainerAddresses(envcontext.WithoutCredentialInvalidator(ctx))
		if err != nil {
			return containermanager.Config{}, fmt.Errorf(
				"cannot determine if provider supports container addresses when calculating container manager config: %w",
				err,
			)
		}
		if supports {
			rval.NetworkingMethod = containermanager.NetworkingMethodProvider
		}
	}

	return rval, nil
}

func (s *Service) ContainerConfig(ctx context.Context) (params.ContainerConfig, error) {
	result := params.ContainerConfig{}

	modelConfig, err := s.st.GetModelConfigKeyValues(ctx, []string{
		config.AuthorizedKeysKey,
		config.EnableOSRefreshUpdateKey,
		config.EnableOSUpgradeKey,
		config.TypeKey,
		config.SSLHostnameVerificationKey,
		config.HTTPProxyKey, config.HTTPSProxyKey, config.FTPProxyKey, config.NoProxyKey,
		config.JujuHTTPProxyKey, config.JujuHTTPSProxyKey, config.JujuFTPProxyKey, config.JujuNoProxyKey,
		config.AptHTTPProxyKey, config.AptHTTPSProxyKey, config.AptFTPProxyKey, config.AptNoProxyKey,
		config.AptMirrorKey,
		config.SnapHTTPProxyKey, config.SnapHTTPSProxyKey,
		config.SnapStoreAssertionsKey,
		config.SnapStoreProxyKey, config.SnapStoreProxyURLKey,
		config.CloudInitUserDataKey,
		config.ContainerInheritPropertiesKey,
	})
	if err != nil {
		return result, fmt.Errorf(
			"cannot get model config keys when calculating container config: %w",
			err,
		)
	}

	controllerConfig, err := s.st.GetControllerConfigKeyValues(ctx, []string{
		controller.SystemSSHKeys,
	})
	if err != nil {
		return result, fmt.Errorf(
			"cannot get controller config keys when calculating container config: %w",
			err,
		)
	}

	result.AuthorizedKeys = ssh.ConcatAuthorisedKeys(
		modelConfig[config.AuthorizedKeysKey],
		controllerConfig[controller.SystemSSHKeys],
	)

	enableOSRefreshUpdate, _ := strconv.ParseBool(modelConfig[config.EnableOSRefreshUpdateKey])
	enableOSUpgrade, _ := strconv.ParseBool(modelConfig[config.EnableOSUpgradeKey])
	result.UpdateBehavior = &params.UpdateBehavior{
		EnableOSRefreshUpdate: enableOSRefreshUpdate,
		EnableOSUpgrade:       enableOSUpgrade,
	}
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
	result.CloudInitUserData, _ = ensureStringMaps(modelConfig[config.CloudInitUserDataKey])
	result.ContainerInheritProperties = modelConfig[config.ContainerInheritPropertiesKey]
	return result, nil
}

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

// ensureStringMaps takes in a string and returns YAML in a map
// where all keys of any nested maps are strings.
// Copied from github.com/juju/juju/environs/config
func ensureStringMaps(in string) (map[string]any, error) {
	userDataMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(in), &userDataMap); err != nil {
		return nil, errors.Annotate(err, "must be valid YAML")
	}
	out, err := utils.ConformYAML(userDataMap)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}
