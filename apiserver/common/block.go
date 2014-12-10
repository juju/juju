// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

// isOperationBlocked determines if the operation should proceed
// based on configuration parameters that prevent destroy, remove or change
// operations.
func isOperationBlocked(operation Operation, cfg *config.Config) bool {
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

// BlockChecker checks for current blocks if any.
type BlockChecker struct {
	getter EnvironConfigGetter
}

func NewBlockChecker(s EnvironConfigGetter) *BlockChecker {
	return &BlockChecker{s}
}

// ChangeAllowed checks if change block is in place.
// Change block prevents all operations that may change
// current environment in any way from running successfully.
func (c *BlockChecker) ChangeAllowed() error {
	return c.checkBlock(ChangeOperation)
}

// RemoveAllowed checks if remove block is in place.
// Remove block prevents removal of machine, service, unit
// and relation from current environment.
func (c *BlockChecker) RemoveAllowed() error {
	return c.checkBlock(RemoveOperation)
}

// DestroyAllowed checks if destroy block is in place.
// Destroy block prevents destruction of current environment.
func (c *BlockChecker) DestroyAllowed() error {
	return c.checkBlock(DestroyOperation)
}

// checkBlock checks if specified operation must be blocked.
// If it does, the method throws specific error that can be examined
// to stop operation execution.
func (c *BlockChecker) checkBlock(operation Operation) error {
	cfg, err := c.getter.EnvironConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if isOperationBlocked(operation, cfg) {
		return ErrOperationBlocked
	}
	return nil
}
