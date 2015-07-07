// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// DestroySystemArgs holds the arguments for destroying a system.
type DestroySystemArgs struct {
	// EnvTag is the tag of the system to destroy.
	EnvTag string

	// DestroyEnvs specifies whether or not the hosted systems
	// should be destroyed in the DestroySystem call.
	DestroyEnvs bool

	// IgnoreBlocks specifies whether or not to ignore blocks
	// on hosted environments.
	IgnoreBlocks bool
}

// EnvironmentBlockInfo holds information about an environment and its
// current blocks.
type EnvironmentBlockInfo struct {
	Environment

	// Blocks contains a list of the current block types enabled
	// for this environment.
	Blocks []string `json:"blocks"`
}

// EnvironmentBlockInfoList holds information about the blocked environments
// for a system.
type EnvironmentBlockInfoList struct {
	Environments []EnvironmentBlockInfo `json:"environments,omitempty"`
}
