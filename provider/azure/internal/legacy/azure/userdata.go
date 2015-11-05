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

const (
	// The userdata on windows will arrive as CustomData.bin
	// We need to execute that as a powershell script and then remove it
	bootstrapUserdataScript = `#ps1_sysnative
mv C:\AzureData\CustomData.bin C:\AzureData\CustomData.ps1
& C:\AzureData\CustomData.ps1
rm C:\AzureData\CustomData.ps1
`
	bootstrapUserdataScriptFilename = "juju-userdata.ps1"
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
