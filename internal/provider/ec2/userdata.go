// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/providerinit/renderers"
)

type AmazonRenderer struct{}

func (AmazonRenderer) Render(cfg cloudinit.CloudConfig, os ostype.OSType) ([]byte, error) {
	switch os {
	case ostype.Ubuntu, ostype.CentOS:
		return renderers.RenderYAML(cfg, utils.Gzip)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}