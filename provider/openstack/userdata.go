// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	jujuos "github.com/juju/utils/os"
)

type OpenstackRenderer struct{}

func (OpenstackRenderer) EncodeUserdata(udata []byte, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return utils.Gzip(udata), nil
	case jujuos.Windows:
		return renderers.WinEmbedInScript(udata), nil
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
