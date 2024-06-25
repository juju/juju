// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containermanager

import (
	"github.com/juju/juju/core/model"
)

type Config struct {
	ImageMetadataURL         string
	ImageStream              string
	LXDSnapChannel           string
	MetadataDefaultsDisabled bool
	ModelID                  model.UUID
	NetworkingMethod         NetworkingMethod
}

type NetworkingMethod string

const (
	NetworkingMethodProvider  = NetworkingMethod("provider")
	NetworkingMethodLocal     = NetworkingMethod("local")
	NetworkingMethodAuto      = NetworkingMethod("auto")
	NetworkingMethodUndefined = NetworkingMethod("")
)

func (n NetworkingMethod) String() string {
	return string(n)
}
