// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/os"

	"github.com/juju/juju/cloudconfig/providerinit/renderers"
)

type AzureRenderer struct{}

func (AzureRenderer) EncodeUserdata(udata []byte, vers os.OSType) ([]byte, error) {
	switch vers {
	case os.Ubuntu, os.CentOS:
		return renderers.ToBase64(utils.Gzip(udata)), nil
	case os.Windows:
		return renderers.ToBase64(renderers.WinEmbedInScript(udata)), nil
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", vers)
	}
}
