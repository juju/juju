// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"fmt"

	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/utils"
)

// userData returns a zipped cloudinit config.
// TODO(bug 1199847): Some of this work can be shared between providers.
func userData(cfg *cloudinit.MachineConfig) ([]byte, error) {
	cloudcfg, err := cloudinit.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("problem with cloudinit config: %v", err)
	}
	data, err := cloudcfg.Render()
	if err != nil {
		return nil, fmt.Errorf("problem with cloudinit config: %v", err)
	}
	cdata := utils.Gzip(data)
	logger.Debugf("user data; %d bytes", len(cdata))
	return cdata, nil
}

// makeCustomData produces custom data for Azure.  This is a base64-encoded
// zipfile of cloudinit userdata.
func makeCustomData(cfg *cloudinit.MachineConfig) (string, error) {
	zipData, err := userData(cfg)
	if err != nil {
		return "", fmt.Errorf("failure while generating custom data: %v", err)
	}
	encodedData := base64.StdEncoding.EncodeToString(zipData)
	logger.Debugf("base64-encoded custom data: %d bytes", len(encodedData))
	return encodedData, nil
}
