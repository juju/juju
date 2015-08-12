// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	"github.com/juju/juju/version"
)

type OpenstackRenderer struct{}

func (OpenstackRenderer) EncodeUserdata(udata []byte, vers version.OSType) ([]byte, error) {
	switch vers {
	case version.Ubuntu, version.CentOS:
		return utils.Gzip(udata), nil
	case version.Windows:
		return renderers.WinEmbedInScript(udata), nil
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", vers)
	}
}
