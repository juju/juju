// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/state"
)

type BlockGetter interface {
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
}

// BlockCheckerInterface defines methods of BlockChecker.
type BlockCheckerInterface interface {
	ChangeAllowed(context.Context) error
	RemoveAllowed(context.Context) error
	DestroyAllowed(context.Context) error
}

// BlockChecker checks for current blocks if any.
type BlockChecker struct {
	getter BlockGetter
}

func NewBlockChecker(s BlockGetter) *BlockChecker {
	return &BlockChecker{s}
}

// ChangeAllowed checks if change block is in place.
// Change block prevents all operations that may change
// current model in any way from running successfully.
func (c *BlockChecker) ChangeAllowed(ctx context.Context) error {
	return c.checkBlock(state.ChangeBlock)
}

// RemoveAllowed checks if remove block is in place.
// Remove block prevents removal of machine, service, unit
// and relation from current model.
func (c *BlockChecker) RemoveAllowed(ctx context.Context) error {
	if err := c.checkBlock(state.RemoveBlock); err != nil {
		return err
	}
	// Check if change block has been enabled
	return c.checkBlock(state.ChangeBlock)
}

// DestroyAllowed checks if destroy block is in place.
// Destroy block prevents destruction of current model.
func (c *BlockChecker) DestroyAllowed(ctx context.Context) error {
	if err := c.checkBlock(state.DestroyBlock); err != nil {
		return err
	}
	// Check if remove block has been enabled
	if err := c.checkBlock(state.RemoveBlock); err != nil {
		return err
	}
	// Check if change block has been enabled
	return c.checkBlock(state.ChangeBlock)
}

// checkBlock checks if specified operation must be blocked.
// If it does, the method throws specific error that can be examined
// to stop operation execution.
func (c *BlockChecker) checkBlock(blockType state.BlockType) error {
	aBlock, isEnabled, err := c.getter.GetBlockForType(blockType)
	if err != nil {
		return errors.Trace(err)
	}
	if isEnabled {
		return apiservererrors.OperationBlockedError(aBlock.Message())
	}
	return nil
}
