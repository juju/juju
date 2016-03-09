// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	jujuos "github.com/juju/utils/os"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
)

type OpenstackRenderer struct{}

func (OpenstackRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return renderers.RenderYAML(cfg, utils.Gzip)
	case jujuos.Windows:
		return renderers.RenderYAML(cfg, renderers.WinEmbedInScript)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
