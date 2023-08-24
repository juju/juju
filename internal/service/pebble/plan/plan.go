// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plan

type Layer struct {
	Order       int                 `yaml:"-"`
	Label       string              `yaml:"-"`
	Summary     string              `yaml:"summary,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Services    map[string]*Service `yaml:"services,omitempty"`
	Checks      map[string]*Check   `yaml:"checks,omitempty"`
}

type Service struct {
	// Basic details
	Name        string         `yaml:"-"`
	Summary     string         `yaml:"summary,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Startup     ServiceStartup `yaml:"startup,omitempty"`
	Override    Override       `yaml:"override,omitempty"`
	Command     string         `yaml:"command,omitempty"`

	// Service dependencies
	After    []string `yaml:"after,omitempty"`
	Before   []string `yaml:"before,omitempty"`
	Requires []string `yaml:"requires,omitempty"`

	// Options for command execution
	Environment map[string]string `yaml:"environment,omitempty"`
	UserID      *int              `yaml:"user-id,omitempty"`
	User        string            `yaml:"user,omitempty"`
	GroupID     *int              `yaml:"group-id,omitempty"`
	Group       string            `yaml:"group,omitempty"`

	// Auto-restart and backoff functionality
	KillDelay      OptionalDuration         `yaml:"kill-delay,omitempty"`
	OnSuccess      ServiceAction            `yaml:"on-success,omitempty"`
	OnFailure      ServiceAction            `yaml:"on-failure,omitempty"`
	OnCheckFailure map[string]ServiceAction `yaml:"on-check-failure,omitempty"`
	BackoffDelay   OptionalDuration         `yaml:"backoff-delay,omitempty"`
	BackoffFactor  OptionalFloat            `yaml:"backoff-factor,omitempty"`
	BackoffLimit   OptionalDuration         `yaml:"backoff-limit,omitempty"`
}

type ServiceStartup string

const (
	StartupUnknown  ServiceStartup = ""
	StartupEnabled  ServiceStartup = "enabled"
	StartupDisabled ServiceStartup = "disabled"
)

// Override specifies the layer override mechanism (for services and checks).
type Override string

const (
	UnknownOverride Override = ""
	MergeOverride   Override = "merge"
	ReplaceOverride Override = "replace"
)

type ServiceAction string

const (
	ActionUnset    ServiceAction = ""
	ActionRestart  ServiceAction = "restart"
	ActionShutdown ServiceAction = "shutdown"
	ActionIgnore   ServiceAction = "ignore"
)

// Check specifies configuration for a single health check.
type Check struct {
	// Basic details
	Name     string     `yaml:"-"`
	Override Override   `yaml:"override,omitempty"`
	Level    CheckLevel `yaml:"level,omitempty"`

	// Common check settings
	Period    OptionalDuration `yaml:"period,omitempty"`
	Timeout   OptionalDuration `yaml:"timeout,omitempty"`
	Threshold int              `yaml:"threshold,omitempty"`

	// Type-specific check settings (only one of these can be set)
	HTTP *HTTPCheck `yaml:"http,omitempty"`
	TCP  *TCPCheck  `yaml:"tcp,omitempty"`
	Exec *ExecCheck `yaml:"exec,omitempty"`
}

// CheckLevel specifies the optional check level.
type CheckLevel string

const (
	UnsetLevel CheckLevel = ""
	AliveLevel CheckLevel = "alive"
	ReadyLevel CheckLevel = "ready"
)

// HTTPCheck holds the configuration for an HTTP health check.
type HTTPCheck struct {
	URL     string            `yaml:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// TCPCheck holds the configuration for an HTTP health check.
type TCPCheck struct {
	Port int    `yaml:"port,omitempty"`
	Host string `yaml:"host,omitempty"`
}

// ExecCheck holds the configuration for an exec health check.
type ExecCheck struct {
	Command     string            `yaml:"command,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	UserID      *int              `yaml:"user-id,omitempty"`
	User        string            `yaml:"user,omitempty"`
	GroupID     *int              `yaml:"group-id,omitempty"`
	Group       string            `yaml:"group,omitempty"`
	WorkingDir  string            `yaml:"working-dir,omitempty"`
}
