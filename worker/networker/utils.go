// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"net"

	"github.com/juju/utils/exec"
)

// Functions defined here for easier patching when testing.
var (
	ExecuteCommands     = executeCommands
	InterfaceIsUp       = interfaceIsUp
	InterfaceHasAddress = interfaceHasAddress
)

// executeCommands execute a batch of commands one by one.
func executeCommands(commands []string) error {
	for _, command := range commands {
		result, err := exec.RunCommands(exec.RunParams{
			Commands:   command,
			WorkingDir: "/",
		})
		if err != nil {
			err := fmt.Errorf("failed to execute %q: %v", command, err)
			logger.Errorf("%s", err.Error())
			return err
		}
		if result.Code != 0 {
			err := fmt.Errorf("command %q failed (code: %d, stdout: %s, stderr: %s)",
				command, result.Code, result.Stdout, result.Stderr)
			logger.Errorf("%s", err.Error())
			return err
		}
	}
	return nil
}

// interfaceIsUp verifies that system network interface is up.
func interfaceIsUp(ifaceName string) bool {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		logger.Errorf("cannot find network interface %q: %v", ifaceName, err)
		return false
	}
	return (iface.Flags & net.FlagUp) != 0
}

// interfaceHasAddress verify that system network interface has at least one assigned address.
func interfaceHasAddress(ifaceName string) bool {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		logger.Errorf("cannot find network interface %q: %v", ifaceName, err)
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		logger.Errorf("cannot get addresses for network interface %q: %v", ifaceName, err)
		return false
	}
	return len(addrs) != 0
}
