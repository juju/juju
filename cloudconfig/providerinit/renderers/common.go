// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package renderers

import (
	"encoding/base64"
	"fmt"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/utils"
)

// ToBase64 just transforms whatever userdata it gets to base64 format
func ToBase64(data []byte) []byte {
	buf := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(buf, data)
	return buf
}

// WinEmbedInScript for now is used on windows and it returns a powershell script
// which has the userdata embedded as base64(gzip(userdata))
func WinEmbedInScript(udata []byte) []byte {
	encUserdata := ToBase64(utils.Gzip(udata))
	return []byte(fmt.Sprintf(cloudconfig.UserdataScript, encUserdata))
}

// AddPowershellTags adds <powershell>...</powershell> to it's input
func AddPowershellTags(udata []byte) []byte {
	return []byte(`<powershell>` +
		string(udata) +
		`</powershell>`)
}
