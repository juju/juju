// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	jujuos "github.com/juju/utils/os"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
)

type lxdRenderer struct{}

// EncodeUserdata implements renderers.ProviderRenderer.
func (lxdRenderer) Render(cfg cloudinit.CloudConfig, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return renderers.RenderYAML(cfg)
	default:
		return nil, errors.Errorf("cannot encode userdata for OS %q", os)
	}
}
