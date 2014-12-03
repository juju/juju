package common

import "github.com/juju/juju/environs/config"

// isOperationBlocked determines if the operation should proceed
// based on configuration parameters that prevent destroy, remove or change
// operations.
//
//              prevent-destroy-on    prevent-remove-on    prevent-change-on
// destroy-op        yes                  yes                 yes
// remove-op         no                   yes                 yes
// change-op         no                   no                  yes
//
//
// If configuration cannot be retrieved, the method assumes the worst
// and blocks operation.
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

type Operation int8

const (
	// Operation that destroys an environment
	DestroyOperation Operation = iota

	// Operation that removes machine, service, unit or relation
	RemoveOperation

	// Operation that changes environments -
	// all adds, modifies, removes, etc
	ChangeOperation
)
