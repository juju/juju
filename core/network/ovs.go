// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"os/exec"
	"strings"

	"github.com/juju/collections/set"

	"github.com/juju/juju/internal/errors"
)

// Overridden by tests
var getCommandOutput = func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

// OvsManagedBridges returns a filtered version of ifaceList that only contains
// bridge interfaces managed by openvswitch.
func OvsManagedBridgeInterfaces(ifaceList InterfaceInfos) (InterfaceInfos, error) {
	ovsBridges, err := OvsManagedBridges()
	if err != nil {
		return nil, errors.Capture(err)
	}

	return ifaceList.Filter(func(iface InterfaceInfo) bool {
		return ovsBridges.Contains(iface.InterfaceName)
	}), nil
}

// OvsManagedBridges returns a set containing the names of all bridge
// interfaces that are managed by openvswitch.
func OvsManagedBridges() (set.Strings, error) {
	haveOvsCli, err := ovsToolsAvailable()
	if err != nil {
		return nil, errors.Capture(err)
	} else if !haveOvsCli { // nothing to do if the tools are missing
		return nil, nil
	}

	// Query list of ovs-managed device names
	res, err := getCommandOutput(exec.Command("ovs-vsctl", "list-br"))
	if err != nil {
		return nil, errors.Errorf("querying ovs-managed bridges via ovs-vsctl: %w", err)
	}

	ovsBridges := set.NewStrings()
	for _, iface := range strings.Split(string(res), "\n") {
		if iface = strings.TrimSpace(iface); iface != "" {
			ovsBridges.Add(iface)
		}
	}
	return ovsBridges, nil
}

func ovsToolsAvailable() (bool, error) {
	if _, err := exec.LookPath("ovs-vsctl"); err != nil {
		// OVS tools not installed
		if execErr, isExecErr := err.(*exec.Error); isExecErr && execErr.Unwrap() == exec.ErrNotFound {
			return false, nil
		}

		return false, errors.Errorf("looking for ovs-vsctl: %w", err)
	}

	return true, nil
}
