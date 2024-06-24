// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containermanager

import (
	"github.com/juju/juju/core/model"
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
