// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/collections/set"
	"github.com/juju/version/v2"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/upgrades/upgradevalidation"
	jujuversion "github.com/juju/juju/version"
)

func checkClientVersion(userLogin bool, callerVersion version.Number) func(facadeName, methodName string) error {
	return func(facadeName, methodName string) error {
		serverVersion := jujuversion.Current
		incompatibleClientError := &params.IncompatibleClientError{
			ServerVersion: serverVersion,
		}
		// If client or server versions are more than one major version apart,
		// reject the call immediately.
		if callerVersion.Major < serverVersion.Major-1 || callerVersion.Major > serverVersion.Major+1 {
			return incompatibleClientError
		}
		// Connection pings always need to be allowed.
		if facadeName == "Pinger" && methodName == "Ping" {
			return nil
		}

		if !userLogin {
			// Only recent older agents can make api calls.
			if minAgentVersion, ok := upgradevalidation.MinAgentVersions[serverVersion.Major]; !ok || callerVersion.Compare(minAgentVersion) < 0 {
				logger.Warningf("rejected agent api all %v.%v for agent version %v", facadeName, methodName, callerVersion)
				return incompatibleClientError
			}
			return nil
		}

		// Clients can still access the "X+1.0" controller facades.
		// But we never allow unfetted access to older controllers
		// as newer clients may have had backwards compatibility removed.
		if callerVersion.Major < serverVersion.Major && serverVersion.Minor == 0 {
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
		if isMethodAllowedForMigrate(facadeName, methodName) {
			return nil
		}

		// Older clients are only allowed to connect to a server if it is
		// within the min client versions map.
		olderClient := callerVersion.Major < serverVersion.Major
		if olderClient {
			if minClientVersion, ok := upgradevalidation.MinClientVersions[serverVersion.Major]; ok && callerVersion.Compare(minClientVersion) >= 0 {
				return nil
			}
			return incompatibleClientError
		}

		// Some calls are needed for upgrades.
		if isMethodAllowedForUpgrade(facadeName, methodName) {
			return nil
		}

		// Very new clients are rejected outright if not otherwise whitelisted
		// above.
		veryNewCaller := callerVersion.Major > serverVersion.Major && callerVersion.Minor != 0
		if veryNewCaller {
			return incompatibleClientError
		}

		// Newer clients with a 0 minor version can only connect to a server if
		// it is recent enough.
		if minServerVersion, ok := upgradevalidation.MinClientVersions[callerVersion.Major]; ok && serverVersion.Compare(minServerVersion) >= 0 {
			return nil
		}

		return incompatibleClientError
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
		"FindTools",
	),
	"ModelUpgrader": set.NewStrings(
		"UpgradeModel",
		"AbortModelUpgrade",
	),
	"ModelConfig": set.NewStrings(
		"ModelGet",
	),
	"Controller": set.NewStrings(
		"ModelConfig",
		"LegacyControllerConfig",
		"ControllerVersion",
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
	"ModelManager": set.NewStrings(
		"ListModels",
		"ModelInfo"),
	"Controller": set.NewStrings(
		"InitiateMigration",
		"IdentityProviderURL",
	),
}
