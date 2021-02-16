// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"net"
	"path/filepath"
)

var netListen = net.Listen

// SupportsIPv6 reports whether the platform supports IPv6 networking
// functionality.
//
// Source: https://github.com/golang/net/blob/master/internal/nettest/stack.go
func SupportsIPv6() bool {
	ln, err := netListen("tcp6", "[::1]:0")
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// GetBridgePorts extracts and returns the names of all interfaces configured as
// ports of the given bridgeName from the Linux kernel userspace SYSFS location
// "<sysPath/<bridgeName>/brif/*". SysClassNetPath should be passed as sysPath.
// Returns an empty result if the ports cannot be determined reliably for any
// reason, or if there are no configured ports for the bridge.
//
// Example call: network.GetBridgePorts(network.SysClassNetPath, "br-eth1")
func GetBridgePorts(sysPath, bridgeName string) []string {
	portsGlobPath := filepath.Join(sysPath, bridgeName, "brif", "*")
	// Glob ignores I/O errors and can only return ErrBadPattern, which we treat
	// as no results, but for debugging we're still logging the error.
	paths, err := filepath.Glob(portsGlobPath)
	if err != nil {
		logger.Debugf("ignoring error traversing path %q: %v", portsGlobPath, err)
	}

	if len(paths) == 0 {
		return nil
	}

	// We need to convert full paths like /sys/class/net/br-eth0/brif/eth0 to
	// just names.
	names := make([]string, len(paths))
	for i := range paths {
		names[i] = filepath.Base(paths[i])
	}
	return names
}
