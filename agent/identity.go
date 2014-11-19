// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/utils"
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
		// Removed the removal code due to race condition in 1.20.12 upgrade.
		// In practice this is unlikely to ever occur as we don't actually
		// support a machine turning from a state server machine into one that
		// no longer does that.  Machines are terminated.
		logger.Infof("would be deleting %q", c.SystemIdentityPath())
	}
	return nil
}
