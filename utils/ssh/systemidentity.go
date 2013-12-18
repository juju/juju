// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"io/ioutil"
	"path/filepath"
)

const systemIdentity = "system-identity"

// WriteSystemIdentity will write the privateKey to the dataDir specified with
// the system identity name.
func WriteSystemIdentity(dataDir string, privateKey string) error {
	filename := SystemIdentityFilename(dataDir)
	logger.Debugf("writing system identity to %s", filename)
	if err := ioutil.WriteFile(filename, []byte(privateKey), 0600); err != nil {
		logger.Errorf("failed writing system identity: %v", err)
		return err
	}
	return nil
}

// SystemIdentityFilename returns the filename for the system identity given a
// data directory.
func SystemIdentityFilename(dataDir string) string {
	return filepath.Join(dataDir, systemIdentity)
}
