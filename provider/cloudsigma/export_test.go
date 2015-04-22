package cloudsigma

import (
	"github.com/juju/juju/environs"
)

func init() {
	environs.RegisterProvider("cloudsigma", providerInstance)
	environs.RegisterImageDataSourceFunc("Image source", getImageSource)
}
