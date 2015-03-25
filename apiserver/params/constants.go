// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// RebootAction defines the action a machine should
// take when a hook needs to reboot
type RebootAction string

const (
	// ShouldDoNothing instructs a machine agent that no action
	// is required on its part
	ShouldDoNothing RebootAction = "noop"
	// ShouldReboot instructs a machine to reboot
	// this happens when a hook running on a machine, requests
	// a reboot
	ShouldReboot RebootAction = "reboot"
	// ShouldShutdown instructs a machine to shut down. This usually
	// happens when running inside a container, and a hook on the parent
	// machine requests a reboot
	ShouldShutdown RebootAction = "shutdown"
)

// ResolvedMode describes the way state transition errors
// are resolved.
type ResolvedMode string

const (
	ResolvedNone       ResolvedMode = ""
	ResolvedRetryHooks ResolvedMode = "retry-hooks"
	ResolvedNoHooks    ResolvedMode = "no-hooks"
)
