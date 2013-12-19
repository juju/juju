// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"io/ioutil"

	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("juju.environs.ssh")

const SystemIdentity = "system-identity"

// WriteSystemIdentity will write the privateKey to the filename specified with
// permissions where only read/write for the owner.
func WriteSystemIdentity(filename string, privateKey string) error {
	logger.Debugf("writing system identity to %s", filename)
	if err := ioutil.WriteFile(filename, []byte(privateKey), 0600); err != nil {
		logger.Errorf("failed writing system identity: %v", err)
		return err
	}
	return nil
}
