// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/juju/environs"
)

func init() {
	environs.RegisterProvider("cloudsigma", providerInstance)
	environs.RegisterImageDataSourceFunc("Image source", getImageSource)
}
