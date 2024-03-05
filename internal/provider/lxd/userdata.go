// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/providerinit/renderers"
)

type lxdRenderer struct{}

// EncodeUserdata implements renderers.ProviderRenderer.
func (lxdRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		bytes, err := renderers.RenderYAML(cfg)
		return bytes, errors.Trace(err)
	default:
		return nil, errors.Errorf("cannot encode userdata for OS %q", os)
	}
}
