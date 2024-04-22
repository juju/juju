// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	b64 "encoding/base64"

	"github.com/juju/errors"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/providerinit/renderers"
)

// OCIRenderer implements the renderers.ProviderRenderer interface
type OCIRenderer struct{}

// Renderer is defined in the renderers.ProviderRenderer interface
func (OCIRenderer) Render(cfg cloudinit.CloudConfig, os ostype.OSType) ([]byte, error) {
	var renderedUdata []byte
	var err error
	switch os {
	case ostype.Ubuntu, ostype.CentOS:
		renderedUdata, err = renderers.RenderYAML(cfg)
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}

	if err != nil {
		return nil, err
	}
	ret := b64.StdEncoding.EncodeToString(renderedUdata)
	return []byte(ret), nil
}
