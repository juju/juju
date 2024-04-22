// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package renderers

import (
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/core/os/ostype"
)

// ProviderRenderer defines a method to encode userdata depending on
// the OS and the provider.
// In the future this might support another method for rendering
// the userdata differently(bash vs yaml) since some providers might
// not ship cloudinit on every OS
type ProviderRenderer interface {
	Render(cloudinit.CloudConfig, ostype.OSType) ([]byte, error)
}
