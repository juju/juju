// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v2/cloudconfig/cloudinit"
	"github.com/juju/juju/v2/cloudconfig/providerinit/renderers"
	jujuos "github.com/juju/juju/v2/core/os"
)

type CloudSigmaRenderer struct{}

func (CloudSigmaRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return renderers.RenderYAML(cfg, renderers.ToBase64)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
