// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
)

// makeCustomData produces custom data for Azure.  This is a base64-encoded
// zipfile of cloudinit userdata.
func makeCustomData(cfg *cloudinit.MachineConfig) (string, error) {
	zipData, err := environs.ComposeUserData(cfg, nil)
	if err != nil {
		return "", fmt.Errorf("failure while generating custom data: %v", err)
	}
	logger.Debugf("user data; %d bytes", len(zipData))
	encodedData := base64.StdEncoding.EncodeToString(zipData)
	logger.Debugf("base64-encoded custom data: %d bytes", len(encodedData))
	return encodedData, nil
}
