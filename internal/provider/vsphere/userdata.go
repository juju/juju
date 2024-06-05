// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/providerinit/renderers"
)

type VsphereRenderer struct{}

func (VsphereRenderer) Render(cfg cloudinit.CloudConfig, os ostype.OSType) ([]byte, error) {
	if os != ostype.Ubuntu {
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os)
	}
	return renderers.RenderYAML(cfg, renderers.ToBase64)
}
