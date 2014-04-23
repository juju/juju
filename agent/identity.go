// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/utils"
)

func WriteSystemIdentityFile(c Config) error {
	info, ok := c.StateServingInfo()
	if !ok {
		return fmt.Errorf("StateServingInfo not available and we need it")
	}
	// Write non-empty contents to the file, otherwise delete it
	if info.SystemIdentity != "" {
		err := utils.AtomicWriteFile(c.SystemIdentityPath(), []byte(info.SystemIdentity), 0600)
		if err != nil {
			return fmt.Errorf("cannot write system identity: %v", err)
		}
	} else {
		os.Remove(c.SystemIdentityPath())
	}
	return nil
}
