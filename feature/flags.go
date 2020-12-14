// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package feature package defines the names of the current feature flags.
package feature

// TODO (anastasiamac 2015-03-02)
// Features that have commands that can be blocked,
// command list for "juju block" and "juju unblock"
// needs to be maintained until we can dynamically discover
// these commands.

// LogErrorStack is a developer feature flag to have the LoggedErrorStack
// function in the utils package write out the error stack as defined by the
// errors package to the logger.  The ability to log the error stack is very
// useful in those error cases where you really don't expect there to be a
// failure.  This means that the developers with this flag set will see the
// stack trace in the log output, but normal deployments never will.
const LogErrorStack = "log-error-stack"

// LegacyUpstart is used to indicate that the version-based init system
// discovery code (service.VersionInitSystem) should return upstart
// instead of systemd for vivid and newer.
const LegacyUpstart = "legacy-upstart"

// DeveloperMode allows access to developer specific commands and behaviour.
const DeveloperMode = "developer-mode"

// StrictMigration will cause migration to error if there are unexported
// values for annotations, status, status history, or settings.
const StrictMigration = "strict-migration"

// Branches will allow for model branches functionality to be used.
const Branches = "branches"

// Generations will allow for model generation functionality to be used.
// This is a deprecated flag name and is synonymous with "branches" above.
const Generations = "generations"

// MongoDbSnap tells Juju to install MongoDB as a snap, rather than installing
// it from APT.
const MongoDbSnap = "mongodb-snap"

// MongoDbSSTXN tells Juju to use server-side transactions. It does nothing if
// MongoDbSnap is not also enabled.
const MongoDbSSTXN = "mongodb-sstxn"

// K8sOperators indicates that it's allowed to deploy charms with mode=operator
const K8sOperators = "k8s-operators"

// RawK8sSpec indicates that it's allowed to set k8s spec using raw yaml format.
const RawK8sSpec = "raw-k8s-spec"

// ActionsV2 enables the next generation actions UX.
const ActionsV2 = "actions-v2"
