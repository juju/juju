// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "github.com/juju/juju/environs/config"

// IsOperationBlocked determines if the operation should proceed
// based on configuration parameters that prevent destroy, remove or change
// operations.
func IsOperationBlocked(operation Operation, cfg *config.Config) bool {
	allChanges := cfg.PreventAllChanges()
	// If all changes are blocked, requesting operation makes no difference
	if allChanges {
		return true
	}

	allRemoves := cfg.PreventRemoveObject()
	// This only matters for Destroy and Remove operations
	if allRemoves && operation != ChangeOperation {
		return true
	}

	allDestroys := cfg.PreventDestroyEnvironment()
	if allDestroys && operation == DestroyOperation {
		return true
	}
	return false
}

// Operation specifies operation type for enum benefit.
// Operation type may be relevant for a group of commands.
type Operation int8

const (
	// DestroyOperation type groups commands that destroy environment.
	DestroyOperation Operation = iota

	// RemoveOperation type groups commands
	// that removes machine, service, unit or relation.
	RemoveOperation

	// ChangeOperation type groups commands that change environments -
	// all adds, modifies, removes, etc.
	ChangeOperation
)
