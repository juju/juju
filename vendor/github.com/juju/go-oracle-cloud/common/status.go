// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package common

type InstanceState string

const (
	StateQueued       InstanceState = "queued"
	StatePreparing    InstanceState = "preparing"
	StateInitializing InstanceState = "initializing"
	StateStarting     InstanceState = "starting"
	StateRunning      InstanceState = "running"
	StateSuspending   InstanceState = "suspending"
	StateSuspended    InstanceState = "suspended"
	StateStopping     InstanceState = "stopping"
	StateStopped      InstanceState = "stopped"
	StateUnreachable  InstanceState = "unreachable"
	StateError        InstanceState = "error"
)

type VolumeState string

const (
	VolumeInitializint VolumeState = "Initializing"
	VolumeDeleting     VolumeState = "Deleting"
	VolumeOnline       VolumeState = "Online"
	VolumeError        VolumeState = "Error"
)
