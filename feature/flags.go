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
// This value is only checked using the controller config "features" attrubite.
const OldPresence = "old-presence"

// DisableRaft will prevent the raft workers from running. At the
// moment the raft cluster isn't managing leadership, so we want the
// ability to stop the workers from running if they cause any issues
// (or just unwanted noise).
const DisableRaft = "disable-raft"

// LegacyLeases will switch all lease management to be handled by the
// Mongo-based lease store, rather than by the Raft FSM.
const LegacyLeases = "legacy-leases"

// LXDProfile will allow for lxd-profile.yaml files in a charm to be used
// in container creation.
const LXDProfile = "lxd-profile"
