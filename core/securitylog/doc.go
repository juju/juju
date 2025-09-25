// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package securitylog provides structured security event logging for Juju.
//
// This package implements security event logging to help detect and monitor
// suspicious activities within the Juju system. Security events are logged
// as structured JSON data with consistent formatting to enable automated
// analysis and alerting.
//
// Supported user-related security events:
//   - User creation (action "created")
//   - User deletion (action "deleted")
//   - User updates (action "updated")
//
// Supported administrative security events:
//   - Changes to user privileges or roles
//
// Supported system security events:
//   - System startup
//   - System shutdown
//   - System crashes
//   - System restarts
//   - Successful user logins
//   - Password changes
//
// Use the universal LogUser function for both creation and update events. The
// legacy LogUserCreated helper is retained for backwards compatibility but
// should be considered deprecated.
//
// Example usage:
//
//	import "github.com/juju/juju/core/securitylog"
//
//	// Log a user creation event
//	securitylog.LogUser(securitylog.UserSecurityEvent{
//		Actor:  "admin",        // the user performing the action
//		Target: "newuser",      // the user being created
//		Action: "created",
//		Access: "login",
//		AppID:  "juju.controller",
//	})
//
// Creation events produce a log entry like:
//
//	{"datetime":"2025-01-01T01:01:01Z","appid":"juju.controller","event":"user_created:admin,newuser:login","level":"WARN","description":"User admin created user newuser with login access level"}
package securitylog
