// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package securitylog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/juju/loggo/v2"
)

var logger = loggo.GetLogger("juju.security")

// SecurityEvent represents a security event log entry
type SecurityEvent struct {
	// DateTime is the event timestamp in UTC
	DateTime time.Time `json:"datetime"`
	// AppID is the application ID, e.g., juju.controller
	AppID string `json:"appid"`
	// Event is the event type and details
	Event string `json:"event"`
	// Level is the log level: INFO, WARN, CRITICAL
	Level string `json:"level"`
	// Description is a human-readable description of the event
	Description string `json:"description"`
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
		return AuthzActionUnknown
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
	// Actor is the user performing the action
	Actor string `json:"actor"`
	// Target is the user being created / deleted
	Target string `json:"target"`
	// Action is the user management action performed
	Action UserManagementAction `json:"action"`
	// Access is the access level granted (e.g., "login", "admin", "no")
	Access string `json:"access"`
	// AppID is the optional application ID (defaults to juju.controller)
	AppID string `json:"app_id"`
	// Level is the optional log level (default INFO)
	Level string `json:"level"`
}

// AuthzSecurityEvent represents an administrative activity security event.
type AuthzSecurityEvent struct {
	// Actor is the administrator performing the action
	Actor string `json:"actor"`
	// Target is the user/resource being modified
	Target string `json:"target"`
	// Action is the authorization action performed
	Action AuthzAction `json:"action"`
	// NewLevel is the new privilege level (for grant/revoke actions)
	NewLevel string `json:"new_level"`
	// AppID is the optional application ID (defaults to juju.controller)
	AppID string `json:"app_id"`
	// Level is optional log level override (default WARN)
	Level string `json:"level"`
}

// PasswordChangeSecurityEvent represents a password change security event.
type PasswordChangeSecurityEvent struct {
	// User is the user whose password was changed
	User string `json:"user"`
	// AppID is the optional application ID (defaults to juju.controller)
	AppID string `json:"app_id"`
	// Level is optional log (default INFO)
	Level string `json:"level"`
}

// LoginSuccessSecurityEvent represents a successful authentication event.
type LoginSuccessSecurityEvent struct {
	// User is the user or service account that authenticated
	User string `json:"user"`
	// AppID is the optional application ID (defaults to juju.controller)
	AppID string `json:"app_id"`
	// Level is optional log level (defaults to INFO)
	Level string `json:"level"`
}

// SystemLifecycleSecurityEvent represents system lifecycle actions (startup, shutdown, restart).
type SystemLifecycleSecurityEvent struct {
	// Actor is the user that initiated the action
	Actor string `json:"actor"`
	// Event is the system lifecycle event: startup, shutdown, restart, crash
	Event SystemLifecycleEvent `json:"event"`
	// Reason is an optional reason for the event
	Reason string `json:"reason"`
	// AppID is the optional application ID (defaults to juju.controller)
	AppID string `json:"app_id"`
	// Level is the optional log level override (default WARN)
	Level string `json:"level"`
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
		eventStr = fmt.Sprintf("%s:%s,%s:%s", string(SecurityUserCreated), ev.Actor, ev.Target, access)
		description = fmt.Sprintf("User %s created user %s with %s access level", ev.Actor, ev.Target, access)
	case UserActionDeleted:
		eventStr = fmt.Sprintf("%s:%s,%s:%s", string(SecurityUserDeleted), ev.Actor, ev.Target, access)
		description = fmt.Sprintf("User %s deleted user %s with %s access level", ev.Actor, ev.Target, access)
	default:
		// Should not happen due to earlier normalization, but just in case
		eventStr = fmt.Sprintf("%s:%s,%s:%s", string(SecurityUserCreated), ev.Actor, ev.Target, access)
		description = fmt.Sprintf("User %s updated user %s with %s access level", ev.Actor, ev.Target, access)
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
	eventStr := fmt.Sprintf("%s:%s,%s", string(SecurityAuthzAdmin), actor, string(action))

	var description string
	switch action {
	case AuthzActionGrant:
		description = fmt.Sprintf("Administrator %s has updated privileges of user %s to %s", actor, ev.Target, newLevel)
	case AuthzActionRevoke:
		description = fmt.Sprintf("Administrator %s has revoked %s privileges of user %s", actor, newLevel, ev.Target)
	case AuthzActionEnable:
		description = fmt.Sprintf("Administrator %s has enabled user %s", actor, ev.Target)
	case AuthzActionDisable:
		description = fmt.Sprintf("Administrator %s has disabled user %s", actor, ev.Target)
	case AuthzActionFailed:
		description = fmt.Sprintf("Administrator %s attempted to access a resource without entitlement", actor)
	default:
		description = fmt.Sprintf("Administrator %s performed action: %s", actor, string(action))
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

	// Event string follows the documented format: authn_password_change:<user>
	eventStr := fmt.Sprintf("%s:%s", string(SecurityPasswordChange), user)
	description := fmt.Sprintf("User %s has successfully changed their password", user)

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

	// Event string follows the documented format: authn_login_success:<user>
	eventStr := fmt.Sprintf("%s:%s", string(SecurityAuthnLoginSuccess), user)
	description := fmt.Sprintf("User %s has successfully logged in", user)

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

	// event string follows the documented format: sys_<event>:<actor>
	var eventStr string
	var description string
	switch ev.Event {
	case SystemLifecycleEventStartup:
		eventStr = fmt.Sprintf("%s:%s", string(SecuritySysStartup), actor)
		description = fmt.Sprintf("User %s spawned a new instance", actor)
	case SystemLifecycleEventShutdown:
		eventStr = fmt.Sprintf("%s:%s", string(SecuritySysShutdown), actor)
		description = fmt.Sprintf("User %s stopped this instance", actor)
	case SystemLifecycleEventRestart:
		eventStr = fmt.Sprintf("%s:%s", string(SecuritySysRestart), actor)
		description = fmt.Sprintf("User %s initiated a restart", actor)
	case SystemLifecycleEventCrash:
		reason := ev.Reason
		if reason == "" {
			reason = "unknown_reason"
		}
		eventStr = fmt.Sprintf("%s:%s", string(SecuritySysCrash), reason)
		description = fmt.Sprintf("The system crashed due to %s", reason)
	default:
		eventStr = fmt.Sprintf("unknown_event:%s", actor)
		description = fmt.Sprintf("An unknown system event was logged by %s", actor)
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
