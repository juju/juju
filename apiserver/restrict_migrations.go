// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/collections/set"

	"github.com/juju/juju/rpc/params"
)

func migrationClientMethodsOnly(facadeName, methodName string) error {
	if !IsMethodAllowedDuringMigration(facadeName, methodName) {
		return params.MigrationInProgressError
	}
	return nil
}

func IsMethodAllowedDuringMigration(facadeName, methodName string) bool {
	methods, ok := allowedMethodsDuringMigration[facadeName]
	if !ok {
		return false
	}
	return methods.Contains(methodName)
}

// allowedMethodsDuringUpgrades stores api calls that are not blocked for user
// logins during the migration of the model from one controller to another.
var allowedMethodsDuringMigration = map[string]set.Strings{
	"Client": set.NewStrings(
		"FullStatus", // for "juju status"
	),
	"Storage": set.NewStrings(
		// for "juju status --storage"
		"ListFilesystems",
		"ListVolumes",
		"ListStorageDetails",
	),
	"SSHClient": set.NewStrings( // allow all SSH client related calls
		"PublicAddress",
		"PrivateAddress",
		"BestAPIVersion",
		"AllAddresses",
		"PublicKeys",
		"Proxy",
	),
	"Pinger": set.NewStrings(
		"Ping",
	),
}
