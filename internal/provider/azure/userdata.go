// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	"github.com/juju/juju/core/os/ostype"
)

type AzureRenderer struct{}

func (AzureRenderer) Render(cfg cloudinit.CloudConfig, os ostype.OSType) ([]byte, error) {
	switch os {
	case ostype.Ubuntu:
		return renderers.RenderYAML(cfg, utils.Gzip, renderers.ToBase64)
	case ostype.CentOS:
		return renderers.RenderScript(cfg, renderers.ToBase64)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os)
	}
}
