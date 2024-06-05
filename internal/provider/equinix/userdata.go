// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/providerinit/renderers"
)

type EquinixRenderer struct{}

func (EquinixRenderer) Render(cfg cloudinit.CloudConfig, os ostype.OSType) ([]byte, error) {
	if os != ostype.Ubuntu {
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os)
	}
	return renderers.RenderYAML(cfg)
}
