// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	jujuos "github.com/juju/juju/core/os"
)

type MAASRenderer struct{}

func (MAASRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return renderers.RenderYAML(cfg, utils.Gzip, renderers.ToBase64)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
