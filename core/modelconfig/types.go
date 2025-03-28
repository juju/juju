// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// ContainerNetworkingMethod defined a strong type for setting and reading the
// model config value for container networking method.
type ContainerNetworkingMethod string

const (
	// ContainerNetworkingMethodLocal sets and indicates that the operator of
	// the model has deemed that the local method be used for all container
	// networking within the model.
	ContainerNetworkingMethodLocal = ContainerNetworkingMethod("local")

	// ContainerNetworkingMethodProvider sets and indicates that the operator of
	// the model has deemed that the provider method be used for all
	// container networking within the model.
	ContainerNetworkingMethodProvider = ContainerNetworkingMethod("provider")

	// ContainerNetworkingMethodAuto set and indicates that the operator of
	// the model has deemed that the Juju controller should determine the best
	// container networking method for the model based on the cloud
	// that is in use.
	ContainerNetworkingMethodAuto = ContainerNetworkingMethod("")
)

// String implements the stringer interface returning a human readable string
// representation of the container networking method.
func (c ContainerNetworkingMethod) String() string {
	return string(c)
}

// Validate checks that the value of [ContainerNetworkingMethod] is an
// understood value by the system. If the value is not valid an error satisfying
// [errors.NotValid] will be returned.
func (c ContainerNetworkingMethod) Validate() error {
	switch c {
	case ContainerNetworkingMethodAuto,
		ContainerNetworkingMethodProvider,
		ContainerNetworkingMethodLocal:
		return nil
	default:
		return errors.Errorf("container networking method value %q %w", c, coreerrors.NotValid)
	}
}
