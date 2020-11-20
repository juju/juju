// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/collections/set"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/params"
	jujuversion "github.com/juju/juju/version"
)

// minAgentMinorVersions defines the minimum minor version
// for a major agent version making api calls to a controller
// with a newer major version.
var minAgentMinorVersions = map[int]int{
	2: 9,
}

func checkClientVersion(userLogin bool, clientVersion version.Number) func(facadeName, methodName string) error {
	return func(facadeName, methodName string) error {
		incompatibleClientError := &params.IncompatibleClientError{
			ServerVersion: jujuversion.Current,
		}
		// If client or server versions are more than one major version apart,
		// reject the call immediately.
		if clientVersion.Major < jujuversion.Current.Major-1 || clientVersion.Major > jujuversion.Current.Major+1 {
			return incompatibleClientError
		}
		// Connection pings always need to be allowed.
		if facadeName == "Pinger" && methodName == "Ping" {
			return nil
		}

		if !userLogin {
			// Only recent older agents can make api calls.
			if minAgentVersion, ok := minAgentMinorVersions[clientVersion.Major]; !ok || minAgentVersion > clientVersion.Minor {
				logger.Debugf("rejected agent api all %v.%v for agent version %v", facadeName, methodName, clientVersion)
				return incompatibleClientError
			}
			return nil
		}

		// Calls to manage the migration of the target controller
		// always need to be allowed.
		if facadeName == "MigrationTarget" {
			return nil
		}
		// Some calls like status we will support always.
		if isMethodAllowedForDifferentClients(facadeName, methodName) {
			return nil
		}

		// The migration worker makes calls masquerading as a user
		// so we need to treat those separately.
		olderClient := clientVersion.Major < jujuversion.Current.Major
		validMigrationCall := isMethodAllowedForMigrate(facadeName, methodName)
		if olderClient && !validMigrationCall {
			return incompatibleClientError
		}

		// Only allow calls to facilitate upgrades or migrations.
		if !validMigrationCall && !isMethodAllowedForUpgrade(facadeName, methodName) {
			return incompatibleClientError
		}
		return nil
	}
}

func isMethodAllowedForDifferentClients(facadeName, methodName string) bool {
	methods, ok := allowedDifferentClientMethods[facadeName]
	if !ok {
		return false
	}
	return methods.Contains(methodName)
}

func isMethodAllowedForUpgrade(facadeName, methodName string) bool {
	upgradeOK := false
	upgradeMethods, ok := allowedMethodsForUpgrade[facadeName]
	if ok {
		upgradeOK = upgradeMethods.Contains(methodName)
	}
	return upgradeOK
}

func isMethodAllowedForMigrate(facadeName, methodName string) bool {
	migrateOK := false
	migrateMethods, ok := allowedMethodsForMigrate[facadeName]
	if ok {
		migrateOK = migrateMethods.Contains(methodName)
	}
	return migrateOK
}

// These methods below are potentially called from a client with
// a different major version to the controller.
// As such we need to ensure we retain compatibility.

// allowedDifferentClientMethods stores api calls we want to
// allow regardless of client or controller version.
var allowedDifferentClientMethods = map[string]set.Strings{
	"Client": set.NewStrings(
		"FullStatus",
	),
}

// allowedMethodsForUpgrade stores api calls
// that are not blocked when the connecting client has
// a major version greater than that of the controller.
var allowedMethodsForUpgrade = map[string]set.Strings{
	"Client": set.NewStrings(
		"SetModelAgentVersion",
		"FindTools",
		"AbortCurrentUpgrade",
	),
	"ModelManager": set.NewStrings(
		"ValidateModelUpgrades",
	),
	"ModelConfig": set.NewStrings(
		"ModelGet",
	),
	"Controller": set.NewStrings(
		"ModelConfig",
		"ControllerConfig",
		"CloudSpec",
	),
}

// allowedMethodsForMigrate stores api calls
// that are not blocked when the connecting client has
// a major version greater than that of the controller.
var allowedMethodsForMigrate = map[string]set.Strings{
	"UserManager": set.NewStrings(
		"UserInfo",
	),
	"Controller": set.NewStrings(
		"InitiateMigration",
		"IdentityProviderURL",
	),
}
