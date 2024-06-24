// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/juju/errors"

	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
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
	// GetModelConfigKeyValues
	GetModelConfigKeyValues(context.Context, []string) (map[string]string, error)
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
