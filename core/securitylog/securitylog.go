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
	SecurityAuthzAdmin         SecurityEventType = "authz_admin"
	SecurityAuthzFail          SecurityEventType = "authz_fail"
	SecurityUserCreated        SecurityEventType = "user_created"
	SecurityUserDeleted        SecurityEventType = "user_deleted"
	SecurityUserUpdated        SecurityEventType = "user_updated"
	SecuritySysMonitorDisabled SecurityEventType = "sys_monitor_disabled"
	SecuritySysShutdown        SecurityEventType = "sys_shutdown"
	SecuritySysStartup         SecurityEventType = "sys_startup"
)

// UserSecurityEvent represents a generic user-related security event (creation or update).
// Action should be one of: "created", "deleted".
type UserSecurityEvent struct {
	Actor  string `json:"actor"`  // The user performing the action
	Target string `json:"target"` // The user being created / deleted
	Action string `json:"action"` // "created" or "deleted"
	Access string `json:"access"` // Access level granted
	AppID  string `json:"app_id"`
	Level  string `json:"level"` // Optional log level override
}

// AuthzEvent represents an administrative activity security event.
type AuthzEvent struct {
	Actor         string `json:"actor"`          // The administrator performing the action
	Target        string `json:"target"`         // The user/resource being modified
	Action        string `json:"action"`         // The administrative action performed
	PreviousLevel string `json:"previous_level"` // Previous privilege level
	NewLevel      string `json:"new_level"`      // New privilege level
	AppID         string `json:"app_id"`
	Level         string `json:"level"` // Optional log level override (default WARN)
}

// LogUser logs a user creation or deletion security event in a unified way.
func LogUser(ev UserSecurityEvent) {
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

	action := strings.ToLower(ev.Action)
	if action != "created" && action != "deleted" {
		action = "updated"
	}

	var eventStr string
	var description string
	switch action {
	case "created":
		// Maintain legacy formatting: user_created:actor,target:access
		eventStr = string(SecurityUserCreated) + ":" + ev.Actor + "," + ev.Target + ":" + access
		description = "User " + ev.Actor + " created user " + ev.Target + " with " + access + " access level"
	case "deleted":
		eventStr = string(SecurityUserDeleted) + ":" + ev.Actor + "," + ev.Target + ":" + access + " access level"
		description = "User " + ev.Actor + " deleted user " + ev.Target + ": " + access + " access level"
	default:
		// Should not happen due to earlier normalization, but just in case
		eventStr = string(SecurityUserUpdated) + ":" + ev.Actor + "," + ev.Target + ":" + access + " access level"
		description = "User " + ev.Actor + " updated user " + ev.Target + ": " + access + " access level"
	}

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	logSecurityEvent(securityEvent)
}

// LogAuthz logs an authorization event
func LogAuthz(ev AuthzEvent) {
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

	// Event string follows the documented format: authz_admin:<actor>,<action>
	eventStr := string(SecurityAuthzAdmin) + ":" + actor + "," + action

	var description string
	switch action {
	case "user_privilege_change":
		description = "Administrator " + actor + " has updated privileges of user " + ev.Target + " from " + ev.PreviousLevel + " to " + ev.NewLevel
	default:
		description = "Administrator " + actor + " performed action: " + action
	}

	securityEvent := SecurityEvent{
		DateTime:    time.Now().UTC(),
		AppID:       appID,
		Event:       eventStr,
		Level:       level,
		Description: description,
	}
	logSecurityEvent(securityEvent)
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
