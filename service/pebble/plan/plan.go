// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Plan constructs are copied directly from the Pebble code base to Juju without
// any modification. We perform this copy as the Pebble package does not isolate
// plan types fully and will cause Juju to not compile correctly on systems that
// are not Linux based.
//
// The code can be found at https://github.com/canonical/pebble/internal/plan
//
// Do not modify this file. Instead propose a change against Pebble and then
// copy the changes over.
package plan

const (
	StartupUnknown  ServiceStartup = ""
	StartupEnabled  ServiceStartup = "enabled"
	StartupDisabled ServiceStartup = "disabled"
)

const (
	UnknownOverride Override = ""
	MergeOverride   Override = "merge"
	ReplaceOverride Override = "replace"
)

const (
	ActionUnset    ServiceAction = ""
	ActionRestart  ServiceAction = "restart"
	ActionShutdown ServiceAction = "shutdown"
	ActionIgnore   ServiceAction = "ignore"
)

const (
	UnsetLevel CheckLevel = ""
	AliveLevel CheckLevel = "alive"
	ReadyLevel CheckLevel = "ready"
)

type Plan struct {
	Layers   []*Layer            `yaml:"-"`
	Services map[string]*Service `yaml:"services,omitempty"`
	Checks   map[string]*Check   `yaml:"checks,omitempty"`
}

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
	OnSuccess      ServiceAction            `yaml:"on-success,omitempty"`
	OnFailure      ServiceAction            `yaml:"on-failure,omitempty"`
	OnCheckFailure map[string]ServiceAction `yaml:"on-check-failure,omitempty"`
	BackoffDelay   OptionalDuration         `yaml:"backoff-delay,omitempty"`
	BackoffFactor  OptionalFloat            `yaml:"backoff-factor,omitempty"`
	BackoffLimit   OptionalDuration         `yaml:"backoff-limit,omitempty"`
}

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

type ExecCheck struct {
	Command     string            `yaml:"command,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	UserID      *int              `yaml:"user-id,omitempty"`
	User        string            `yaml:"user,omitempty"`
	GroupID     *int              `yaml:"group-id,omitempty"`
	Group       string            `yaml:"group,omitempty"`
	WorkingDir  string            `yaml:"working-dir,omitempty"`
}

type TCPCheck struct {
	Port int    `yaml:"port,omitempty"`
	Host string `yaml:"host,omitempty"`
}

type HTTPCheck struct {
	URL     string            `yaml:"url,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type CheckLevel string

type Override string

type ServiceAction string

type ServiceStartup string
