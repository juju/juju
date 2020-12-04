// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packet

import (
	"github.com/juju/errors"
	jujuos "github.com/juju/os/v2"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
)

type PacketRenderer struct{}

func (PacketRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu:
		return renderers.RenderScript(cfg)
	// case jujuos.Windows:
	// 	return renderers.RenderYAML(cfg, renderers.WinEmbedInScript)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
