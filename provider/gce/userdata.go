// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	jujuos "github.com/juju/utils/os"

	"github.com/juju/juju/cloudconfig/providerinit/renderers"
)

type GCERenderer struct{}

func (GCERenderer) EncodeUserdata(udata []byte, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return renderers.ToBase64(utils.Gzip(udata)), nil
	case jujuos.Windows:
		return renderers.WinEmbedInScript(udata), nil
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}

// The hostname on windows GCE instances is taken from
// the instance id. This is bad because windows only
// uses the first 15 characters in certain instances,
// which are not unique for the GCE provider.
// As a result, we have to send this small script as
// a sysprep script, to change the hostname inside
// the sysprep step, simplyfing the userdata and
// saving a reboot
var winSetHostnameScript = `
Rename-Computer %q
`
