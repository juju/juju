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
	Interfaces          = interfaces
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
			return fmt.Errorf("failed to execute %q: %v", command, err)
		}
		if result.Code != 0 {
			return fmt.Errorf(
				"command %q failed (code: %d, stdout: %s, stderr: %s)",
				command, result.Code, result.Stdout, result.Stderr)
		}
		logger.Debugf("command %q (code: %d, stdout: %s, stderr: %s)",
			command, result.Code, result.Stdout, result.Stderr)
	}
	return nil
}

// interfaceIsUp returns whether the given network interface is up.
func interfaceIsUp(interfaceName string) bool {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		// Log as warning, because with virtual interfaces, there
		// might be pending commands to execute that actually create
		// the interface first before we can look it up.
		logger.Warningf("cannot tell if %q is up: %v", interfaceName, err)
		return false
	}
	return (iface.Flags & net.FlagUp) != 0
}

// interfaceHasAddress whether the given network interface has at
// least one assigned address.
func interfaceHasAddress(interfaceName string) bool {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		// Log as warning, because with virtual interfaces, there
		// might be pending commands to execute that actually create
		// the interface first before we can look it up.
		logger.Warningf("cannot tell if %q has addresses: %v", interfaceName, err)
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		logger.Errorf("cannot get addresses for network interface %q: %v", interfaceName, err)
		return false
	}
	return len(addrs) != 0
}

// interfaces returns all known network interfaces on the machine.
func interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}
