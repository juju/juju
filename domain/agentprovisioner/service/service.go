// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/ssh"

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
	GetModelConfigKeyValues(context.Context, []string) (*config.Config, error)
	// GetControllerConfigKeyValues returns a controller config object
	// populated with values for the provided keys.
	GetControllerConfigKeyValues(context.Context, []string) (*controller.Config, error)
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
		rval.LXDSnapChannel = cfg.LXDSnapChannel()
	}

	rval.ImageMetadataURL, _ = cfg.ContainerImageMetadataURL()
	rval.MetadataDefaultsDisabled = cfg.ContainerImageMetadataDefaultsDisabled()
	rval.ImageStream = cfg.ContainerImageStream()

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

	cfg, err := s.st.GetModelConfigKeyValues(ctx, []string{
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

	authorizedKeys := ssh.ConcatAuthorisedKeys(
		cfg.AuthorizedKeys(), controllerConfig.SystemSSHKeys())

	result.UpdateBehavior = &params.UpdateBehavior{
		EnableOSRefreshUpdate: cfg.EnableOSRefreshUpdate(),
		EnableOSUpgrade:       cfg.EnableOSUpgrade(),
	}
	result.ProviderType = cfg.Type()
	result.AuthorizedKeys = authorizedKeys
	result.SSLHostnameVerification = cfg.SSLHostnameVerification()
	result.LegacyProxy = cfg.LegacyProxySettings()
	result.JujuProxy = cfg.JujuProxySettings()
	result.AptProxy = cfg.AptProxySettings()
	result.AptMirror = cfg.AptMirror()
	result.SnapProxy = cfg.SnapProxySettings()
	result.SnapStoreAssertions = cfg.SnapStoreAssertions()
	result.SnapStoreProxyID = cfg.SnapStoreProxy()
	result.SnapStoreProxyURL = cfg.SnapStoreProxyURL()
	result.CloudInitUserData = cfg.CloudInitUserData()
	result.ContainerInheritProperties = cfg.ContainerInheritProperties()
	return result, nil
}
