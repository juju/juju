// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/collections/set"

	"github.com/juju/juju/rpc/params"
)

func upgradeMethodsOnly(facadeName, methodName string) error {
	if !IsMethodAllowedDuringUpgrade(facadeName, methodName) {
		return params.UpgradeInProgressError
	}
	return nil
}

func IsMethodAllowedDuringUpgrade(facadeName, methodName string) bool {
	methods, ok := allowedMethodsDuringUpgrades[facadeName]
	if !ok {
		return false
	}
	return methods.Contains(methodName)
}

// allowedMethodsDuringUpgrades stores api calls
// that are not blocked during the upgrade process
// as well as  their respective facade names.
// When needed, at some future point, this solution
// will need to be adjusted to cater for different
// facade versions as well.
var allowedMethodsDuringUpgrades = map[string]set.Strings{
	"Client": set.NewStrings(
		"FullStatus",          // for "juju status"
		"FindTools",           // for "juju upgrade-model", before we can reset upgrade to re-run
		"AbortCurrentUpgrade", // for "juju upgrade-model", so that we can reset upgrade to re-run

	),
	"SSHClient": set.NewStrings( // allow all SSH client related calls
		"PublicAddress",
		"PrivateAddress",
		"BestAPIVersion",
		"AllAddresses",
		"PublicKeys",
		"Proxy",
		"Leader",
		"VirtualHostname",
		"PublicHostKeyForTarget",
	),
	"Pinger": set.NewStrings(
		"Ping",
	),
}
