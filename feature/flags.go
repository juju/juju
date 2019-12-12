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

// PostNetCLIMVP is used to indicate that additional networking
// commands will be available in addition to the networking MVP ones
// (space list|create, subnet list|add).
const PostNetCLIMVP = "post-net-cli-mvp"

// ImageMetadata allows custom image metadata to be recorded in state.
const ImageMetadata = "image-metadata"

// DeveloperMode allows access to developer specific commands and behaviour.
const DeveloperMode = "developer-mode"

// StrictMigration will cause migration to error if there are unexported
// values for annotations, status, status history, or settings.
const StrictMigration = "strict-migration"

// OldPresence indicates that the old database presence implementation
// should be used by the API server to determine agent presence.
// This value is only checked using the controller config "features" attribute.
const OldPresence = "old-presence"

// LegacyLeases will switch all lease management to be handled by the
// Mongo-based lease store, rather than by the Raft FSM.
const LegacyLeases = "legacy-leases"

// Branches will allow for model branches functionality to be used.
const Branches = "branches"

// Generations will allow for model generation functionality to be used.
// This is a deprecated flag name and is synonymous with "branches" above.
const Generations = "generations"

// MongoDbSnap tells Juju to install MongoDB as a snap, rather than installing
// it from APT.
const MongoDbSnap = "mongodb-snap"

// MongoDbSnap tells Juju to use server-side transactions. It does nothing if
// MongoDbSnap is not also enabled.
const MongoDbSSTXN = "mongodb-sstxn"

// MultiCloud tells Juju to allow a different IAAS cloud to the one the controller
// was bootstrapped on to be added to the controller.
const MultiCloud = "multi-cloud"

// JujuV3 indicates that new CLI commands and behaviour for v3 should be enabled.
const JujuV3 = "juju-v3"

// CMRMigrations indicates that cross model relations (CMR) can migrate
// information from one controller to another controller.
// This feature is disabled during import and export of information, turning
// this on will allow that to happen.
const CMRMigrations = "cmr-migrations"
