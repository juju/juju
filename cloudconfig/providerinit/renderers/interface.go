// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The renderers package implements a way to encode the userdata
// depending on the OS and the provider.
// It currently holds an interface and common functions, while
// the implementations live in the particular providers.
package renderers

import (
	"github.com/juju/utils/os"

	"github.com/juju/juju/cloudconfig/cloudinit"
)

// ProviderRenderer defines a method to encode userdata depending on
// the OS and the provider.
// In the future this might support another method for rendering
// the userdata differently(bash vs yaml) since some providers might
// not ship cloudinit on every OS
type ProviderRenderer interface {
	Render(cloudinit.CloudConfig, os.OSType) ([]byte, error)
}
