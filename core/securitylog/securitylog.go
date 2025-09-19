// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package securitylog

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.security")

// SecurityEvent represents a security event log entry
type SecurityEvent struct {
	DateTime    time.Time `json:"datetime"`
	AppID       string    `json:"appid"`
	Event       string    `json:"event"`
	Level       string    `json:"level"`
	Description string    `json:"description"`
}

// SecurityEventType represents the type of security event
type SecurityEventType string

// Defined security event types
const (
	SecurityAuthzAdmin        SecurityEventType = "authz_admin"
	SecurityUserCreated       SecurityEventType = "user_created"
	SecurityUserDeleted       SecurityEventType = "user_deleted"
	SecurityUserUpdated       SecurityEventType = "user_updated"
	SecuritySysShutdown       SecurityEventType = "sys_shutdown"
	SecuritySysStartup        SecurityEventType = "sys_startup"
	SecuritySysRestart        SecurityEventType = "sys_restart"
	SecuritySysCrash          SecurityEventType = "sys_crash"
	SecurityPasswordChange    SecurityEventType = "authn_password_change"
	SecurityAuthnLoginSuccess SecurityEventType = "authn_login_success"
)

// SystemLifecycleEvent represents the type of system lifecycle event
type SystemLifecycleEvent string

// Defined System Lifecycle event types
const (
	SystemLifecycleEventStartup  SystemLifecycleEvent = "startup"
	SystemLifecycleEventShutdown SystemLifecycleEvent = "shutdown"
	SystemLifecycleEventRestart  SystemLifecycleEvent = "restart"
	SystemLifecycleEventCrash    SystemLifecycleEvent = "crash"
)

// Defined default admin name
const DefaultAdminName = "admin"

// AuthzAction represents the type of authorization action performed.
type AuthzAction string

// Defined authorization actions
const (
	AuthzActionGrant   AuthzAction = "grant"
	AuthzActionRevoke  AuthzAction = "revoke"
	AuthzActionEnable  AuthzAction = "enabled"
	AuthzActionDisable AuthzAction = "disabled"
	AuthzActionFailed  AuthzAction = "failed"
	AuthzActionUnknown AuthzAction = "unknown_action"
)

// Define func that define AuthzAction value from string
func ParseAuthzAction(action string) AuthzAction {
	switch strings.ToLower(action) {
	case "grant":
		return AuthzActionGrant
	case "revoke":
		return AuthzActionRevoke
	case "enable":
		return AuthzActionEnable
	case "disable":
		return AuthzActionDisable
	case "failed":
		return AuthzActionFailed
	default:
		return AuthzAction("unknown_action")
	}
}

// UserManagementAction represents the type of user management action performed.
type UserManagementAction string

// Defined user management actions
const (
	UserActionCreated UserManagementAction = "created"
	UserActionDeleted UserManagementAction = "deleted"
	UserActionUpdated UserManagementAction = "updated"
)

// UserSecurityEvent represents a generic user-related security event (creation or update).
// Action should be one of: "created", "deleted".
type UserSecurityEvent struct {
	Actor  string               `json:"actor"`  // The user performing the action
	Target string               `json:"target"` // The user being created / deleted
	Action UserManagementAction `json:"action"` // "created" or "deleted"
	Access string               `json:"access"` // Access level granted
	AppID  string               `json:"app_id"`
	Level  string               `json:"level"` // Optional log level override
}

// AuthzSecurityEvent represents an administrative activity security event.
type AuthzSecurityEvent struct {
	Actor    string      `json:"actor"`     // The administrator performing the action
	Target   string      `json:"target"`    // The user/resource being modified
	Action   AuthzAction `json:"action"`    // The administrative action performed
	NewLevel string      `json:"new_level"` // New privilege level
	AppID    string      `json:"app_id"`
	Level    string      `json:"level"` // Optional log level override (default WARN)
}

// PasswordChangeSecurityEvent represents a password change security event.
type PasswordChangeSecurityEvent struct {
	User  string `json:"user"`   // The user whose password was changed
	AppID string `json:"app_id"` // Optional application ID (defaults to juju.controller)
	Level string `json:"level"`  // Optional log level (defaults to INFO)
}

// LoginSuccessSecurityEvent represents a successful authentication event.
type LoginSuccessSecurityEvent struct {
	User  string `json:"user"`   // The user or service account that authenticated
	AppID string `json:"app_id"` // Optional application ID (defaults to juju.controller)
	Level string `json:"level"`  // Optional log level (defaults to INFO)
}

// SystemLifecycleSecurityEvent represents system lifecycle actions (startup, shutdown, restart).
type SystemLifecycleSecurityEvent struct {
	Actor  string               `json:"actor"`  // The user that initiated the action
	Event  SystemLifecycleEvent `json:"event"`  // The lifecycle event: startup, shutdown, restart, crash
	Reason string               `json:"reason"` // Optional reason for the event (e.g., crash reason)
	AppID  string               `json:"app_id"` // Optional application ID (defaults to juju.controller)
	Level  string               `json:"level"`  // Optional log level (defaults to WARN)
}

// LogUser logs a user creation or deletion security event in a unified way.
func LogUser(ev UserSecurityEvent) {
	securityEvent := formLogUserSecurityEvent(ev)
	logSecurityEvent(securityEvent)
}

// LogAuthz logs an authorization event
func LogAuthz(ev AuthzSecurityEvent) {
	securityEvent := formLogAuthzSecurityEvent(ev)
	logSecurityEvent(securityEvent)
}

// LogPasswordChange logs a password change security event.
func LogPasswordChange(ev PasswordChangeSecurityEvent) {
	securityEvent := formPasswordChangeSecurityEvent(ev)
	logSecurityEvent(securityEvent)
}

// LogLoginSuccess logs a successful login (authentication) event.
func LogLoginSuccess(ev LoginSuccessSecurityEvent) {
	securityEvent := formLoginSuccessSecurityEvent(ev)
	logSecurityEvent(securityEvent)
}

// LogSystem logs a system startup/shutdown/restart/crash events.
func LogSystem(ev SystemLifecycleSecurityEvent) {
	securityEvent := formSystemLifecycleSecurityEvent(ev)
	logSecurityEvent(securityEvent)
}

// formLogUserSecurityEvent creates a SecurityEvent from a UserSecurityEvent
func formLogUserSecurityEvent(ev UserSecurityEvent) SecurityEvent {
	appID := ev.AppID
	if appID == "" {
		appID = "juju.controller"
	}

	level := ev.Level
	if level == "" {
		level = "INFO"
	}

	access := ev.Access
	if access == "" {
		access = "no"
	}

	action := ev.Action
	if action == "" {
		action = UserActionUpdated
	}

	var eventStr string
	var description string
	switch action {
	case UserActionCreated:
		// Maintain legacy formatting: user_created:actor,target:access
		eventStr = string(SecurityUserCreated) + ":" + ev.Actor + "," + ev.Target + ":" + access
		description = "User " + ev.Actor + " created user " + ev.Target + " with " + access + " access level"
	case UserActionDeleted:
		eventStr = string(SecurityUserDeleted) + ":" + ev.Actor + "," + ev.Target + ":" + access
		description = "User " + ev.Actor + " deleted user " + ev.Target + " with " + access + " access level"
	default:
		// Should not happen due to earlier normalization, but just in case
		eventStr = string(SecurityUserUpdated) + ":" + ev.Actor + "," + ev.Target + ":" + access
		description = "User " + ev.Actor + " updated user " + ev.Target + " with " + access + " access level"
	}

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	return securityEvent
}

// formLogAuthzSecurityEvent creates a SecurityEvent from an AuthzSecurityEvent
func formLogAuthzSecurityEvent(ev AuthzSecurityEvent) SecurityEvent {
	// Defaults
	appID := ev.AppID
	if appID == "" {
		appID = "juju.controller"
	}
	level := ev.Level
	if level == "" {
		level = "WARN"
	}
	actor := ev.Actor
	if actor == "" {
		actor = "unknown"
	}
	action := ev.Action
	if action == "" {
		action = "unknown_action"
	}
	newLevel := ev.NewLevel
	if newLevel == "" {
		newLevel = "unknown_level"
	}

	// Event string follows the documented format: authz_admin:<actor>,<action>
	eventStr := string(SecurityAuthzAdmin) + ":" + actor + "," + string(action)

	var description string
	switch action {
	case AuthzActionGrant:
		description = "Administrator " + actor + " has updated privileges of user " + ev.Target + " to " + newLevel
	case AuthzActionRevoke:
		description = "Administrator " + actor + " has revoked " + newLevel + " privileges of user " + ev.Target
	case AuthzActionEnable:
		description = "Administrator " + actor + " has enabled user " + ev.Target
	case AuthzActionDisable:
		description = "Administrator " + actor + " has disabled user " + ev.Target
	case AuthzActionFailed:
		description = "User " + actor + " attempted to access a resource without entitlement"
	default:
		description = "Administrator " + actor + " performed action: " + string(action)
	}

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	return securityEvent
}

// formPasswordChangeSecurityEvent creates a SecurityEvent from a PasswordChangeSecurityEvent
func formPasswordChangeSecurityEvent(ev PasswordChangeSecurityEvent) SecurityEvent {
	// Defaults
	appID := ev.AppID
	if appID == "" {
		appID = "juju.controller"
	}
	level := ev.Level
	if level == "" {
		level = "INFO"
	}
	user := ev.User
	if user == "" {
		user = "unknown"
	}

	// Event string: authn_password_change:<user>
	eventStr := string(SecurityPasswordChange) + ":" + user

	// Description with optional source
	description := "User " + user + " has successfully changed their password"

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	return securityEvent
}

// formLoginSuccessSecurityEvent creates a SecurityEvent from a LoginSuccessSecurityEvent
func formLoginSuccessSecurityEvent(ev LoginSuccessSecurityEvent) SecurityEvent {
	// Defaults
	appID := ev.AppID
	if appID == "" {
		appID = "juju.controller"
	}
	level := ev.Level
	if level == "" {
		level = "INFO"
	}
	user := ev.User
	if user == "" {
		user = "unknown"
	}

	eventStr := string(SecurityAuthnLoginSuccess) + ":" + user

	description := "User " + user + " has successfully logged in"

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	return securityEvent
}

// formSystemLifecycleSecurityEvent creates a SecurityEvent from a SystemLifecycleSecurityEvent
func formSystemLifecycleSecurityEvent(ev SystemLifecycleSecurityEvent) SecurityEvent {
	appID := ev.AppID
	if appID == "" {
		appID = "juju.controller"
	}
	level := ev.Level
	if level == "" {
		level = "WARN"
	}
	actor := ev.Actor
	if actor == "" {
		actor = "unknown"
	}

	var eventStr string
	var description string
	switch ev.Event {
	case SystemLifecycleEventStartup:
		eventStr = string(SecuritySysStartup) + ":" + actor
		description = "User " + actor + " spawned a new instance"
	case SystemLifecycleEventShutdown:
		eventStr = string(SecuritySysShutdown) + ":" + actor
		description = "User " + actor + " stopped this instance"
	case SystemLifecycleEventRestart:
		eventStr = string(SecuritySysRestart) + ":" + actor
		description = "User " + actor + " initiated a restart"
	case SystemLifecycleEventCrash:
		reason := ev.Reason
		if reason == "" {
			reason = "unknown_reason"
		}
		eventStr = string(SecuritySysCrash) + ":" + reason
		description = "The system crashed due to " + reason
	default:
		eventStr = "unknown_event:" + actor
		description = "An unknown system event was logged by " + actor
	}

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	return securityEvent
}

// logSecurityEvent logs a security event as structured JSON
func logSecurityEvent(event SecurityEvent) {
	jsonData, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("Failed to marshal security event: %v", err)
		return
	}

	switch event.Level {
	case "INFO":
		logger.Infof(string(jsonData))
	case "WARN":
		logger.Warningf(string(jsonData))
	case "CRITICAL":
		logger.Criticalf(string(jsonData))
	default:
		logger.Infof(string(jsonData))
	}
}
