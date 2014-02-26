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
	privateKey, publicKey, err := ssh.GenerateKey(config.JujuSystemKeyComment)
	if err != nil {
		return fmt.Errorf("failed to create system key: %v", err)
	}
	// Write new authorised key.
	client := context.APIState().Client()
	cfg, err := client.EnvironmentGet()
	authorised_keys := config.ConcatAuthKeys(cfg[config.AuthKeysConfig].(string), publicKey)
	if err != nil {
		return fmt.Errorf("failed to read current environment config: %v", err)
	}
	err = client.EnvironmentSet(map[string]interface{}{
		config.AuthKeysConfig: authorised_keys,
	})
	if err != nil {
		return fmt.Errorf("failed to update authoised keys with new system key: %v", err)
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
