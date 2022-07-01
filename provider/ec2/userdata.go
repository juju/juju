// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/v2/cloudconfig/cloudinit"
	"github.com/juju/juju/v2/cloudconfig/providerinit/renderers"
	jujuos "github.com/juju/juju/v2/core/os"
)

type AmazonRenderer struct{}

func (AmazonRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return renderers.RenderYAML(cfg, utils.Gzip)
	case jujuos.Windows:
		return renderers.RenderYAML(cfg, renderers.WinEmbedInScript, renderers.AddPowershellTags)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
