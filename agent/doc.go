// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent manages agent configuration for Juju agents.
// See the sections below for details about this package.
//
// See github.com/juju/juju/api for establishing API connections using agent configuration.
// See github.com/juju/juju/controller for controller-specific settings that agents should handle.
//
// # How this package works
//
// **Agents**: Agents are Juju processes that run on behalf of specific entities
// (machines, units, models, or controllers). Agent configuration provides
// persistent identity, credentials, and connection details -- API credentials,
// controller addresses, CA certificates, directory paths, and operational
// settings like logging configuration -- that allow them to authenticate to the
// controller and perform their work.
//
// **Agent configuration persistence**: Configuration files use a versioned format.
// Juju supports reading the current format and the format from the previous
// stable release.
//
// **Agent password bootstrapping**: New agents are created with an old password
// but no current password. When an agent first connects to the API server using
// the old password, it generates a new secure password and saves it via
// SetPassword. The old password then serves as a fallback.
//
//	New Agent                 First Connect              After Connect
//	+-----------------+       +-----------------+       +-----------------+
//	| old: set        |       | old: set        |       | current: NEW    |
//	| current: empty  | ----> | current: empty  | ----> | old: set        |
//	+-----------------+       |                 |       +-----------------+
//	                          | Connect with    |
//	                          | old password    |
//	                          | Generate new    |
//	                          | SetPassword()   |
//	                          +-----------------+
//
// # How to use this package correctly
//
// **Validation**: Callers MUST NOT parse configuration files directly. Use
// ReadConfig instead to ensure version compatibility. Callers MUST NOT assume
// both passwords are populated on a newly created configuration.
package agent
