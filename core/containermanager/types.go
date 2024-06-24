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
	NetworkingMethodProvider = NetworkingMethod("provider")
	NetworkingMethodLocal    = NetworkingMethod("local")
)

func (n NetworkingMethod) String() string {
	return string(n)
}
