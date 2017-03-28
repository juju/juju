// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/juju/errors"
	jujuos "github.com/juju/utils/os"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
)

// OracleRenderer defines a method to encode userdata depending on the
// OS provided if the oracle provider supports it.
// This type implements the renderers.ProviderRenderer
type OracleRenderer struct{}

// Renderer takes a config and os type and returns the correct
// cloud-config YAML. This shall be called from the
// providerinit.ComposeUserdata when passed
// This currently only supports the Ubuntu os type
func (OracleRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu:
		return renderers.RenderYAML(cfg)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
