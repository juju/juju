// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containermanager

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelconfig"
	"github.com/juju/juju/internal/errors"
)

// Config stores the configuration for a container manager
type Config struct {
	ImageMetadataURL         string
	ImageStream              string
	LXDSnapChannel           string
	MetadataDefaultsDisabled bool
	ModelID                  model.UUID
	NetworkingMethod         NetworkingMethod
}

// NetworkingMethod represents a networking method for a container. The options
// are:
//   - provider: the container's networking is handled by the provider;
//   - local: the container's networking is provided by the host machine.
type NetworkingMethod string

const (
	NetworkingMethodProvider = NetworkingMethod("provider")
	NetworkingMethodLocal    = NetworkingMethod("local")
)

// String returns the underlying string representation of a networking method.
func (n NetworkingMethod) String() string {
	return string(n)
}

// ResolveNetworkingMethod resolves the model config value for the container
// networking method into the effective networking method. The "auto" value
// (the unset model config key) is resolved using whether the model's cloud
// provider supports allocating container addresses: providers that do use the
// provider method, and all others use local.
// The supportsContainerAddresses value is only consulted for the "auto"
// value.
func ResolveNetworkingMethod(
	method modelconfig.ContainerNetworkingMethod,
	supportsContainerAddresses bool,
) (NetworkingMethod, error) {
	switch method {
	case modelconfig.ContainerNetworkingMethodLocal:
		return NetworkingMethodLocal, nil
	case modelconfig.ContainerNetworkingMethodProvider:
		return NetworkingMethodProvider, nil
	case modelconfig.ContainerNetworkingMethodAuto:
		if supportsContainerAddresses {
			return NetworkingMethodProvider, nil
		}
		return NetworkingMethodLocal, nil
	default:
		return "", errors.Errorf(
			"unable to deduce container networking method %q from model config", method,
		).Add(coreerrors.NotValid)
	}
}

// ResolveNetworkingMethodWithCapability resolves the effective container
// networking method for a model, given its configured value and a function
// reporting whether the model's cloud provider supports allocating
// container addresses.
// Explicitly configured methods are used as configured and the provider
// capability is never consulted; the unset "auto" value is resolved by
// lazily calling supportsContainerAddresses, which returns false for
// providers that do not implement the networking capability at all.
func ResolveNetworkingMethodWithCapability(
	method modelconfig.ContainerNetworkingMethod,
	supportsContainerAddresses func() (bool, error),
) (NetworkingMethod, error) {
	if method != modelconfig.ContainerNetworkingMethodAuto {
		// An explicitly configured method needs no provider capability
		// to resolve.
		return ResolveNetworkingMethod(method, false)
	}

	supports, err := supportsContainerAddresses()
	if err != nil {
		return "", errors.Capture(err)
	}
	return ResolveNetworkingMethod(method, supports)
}
