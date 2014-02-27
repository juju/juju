// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/keymanager"
	"launchpad.net/juju-core/utils/ssh"
)

func ensureSystemSSHKey(context Context) error {
	identityFile := path.Join(context.AgentConfig().DataDir(), cloudinit.SystemIdentity)
	// Don't generate a key unless we have to.
	keyExists, err := systemKeyExists(identityFile)
	if err != nil {
		return fmt.Errorf("failed to check system key exists: %v", err)
	}
	if keyExists {
		return nil
	}
	privateKey, publicKey, err := ssh.GenerateKey(config.JujuSystemKey)
	if err != nil {
		return fmt.Errorf("failed to create system key: %v", err)
	}
	// Write new authorised key.
	keyManager := keymanager.NewClient(context.APIState())
	errResults, err := keyManager.AddKeys(config.JujuSystemKey, publicKey)
	apiErr := err
	if apiErr == nil {
		apiErr = errResults[0].Error
	}
	if err != nil || errResults[0].Error != nil {
		return fmt.Errorf("failed to update authoised keys with new system key: %v", apiErr)
	}
	return ioutil.WriteFile(identityFile, []byte(privateKey), 0600)
}

func systemKeyExists(identityFile string) (bool, error) {
	_, err := os.Stat(identityFile)
	if err == nil {
		return true, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}
