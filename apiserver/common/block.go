// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/internal/services"
)

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// BlockCheckerInterface defines methods of BlockChecker.
type BlockCheckerInterface interface {
	// ChangeAllowed checks if change block is in place.
	ChangeAllowed(context.Context) error
	// RemoveAllowed checks if remove block is in place.
	RemoveAllowed(context.Context) error
	// DestroyAllowed checks if destroy block is in place.
	DestroyAllowed(context.Context) error
}

// BlockCheckerGetter provides a signature of a function that can be used to
// late defer getting a [BlockChecker]. This allows block checkers to be made
// based on the context of the caller.
type BlockCheckerGetter func(ctx context.Context) (*BlockChecker, error)

// BlockChecker checks for current blocks if any.
type BlockChecker struct {
	service BlockCommandService
}

// DomainServicesGetter describes a type that can be used for getting
// [services.DomainServices] from a given context that comes from a http
// request.
type DomainServicesGetter func(ctx context.Context) (services.DomainServices, error)

// BlockCheckerGetterForServices returns a [BlockCheckerGetter] that is
// constructed from the supplied context.
func BlockCheckerGetterForServices(servicesGetter DomainServicesGetter) BlockCheckerGetter {
	return func(ctx context.Context) (*BlockChecker, error) {
		svc, err := servicesGetter(ctx)
		if err != nil {
			return nil, err
		}

		return NewBlockChecker(svc.BlockCommand()), nil
	}
}

// NewBlockChecker returns a new BlockChecker.
func NewBlockChecker(s BlockCommandService) *BlockChecker {
	return &BlockChecker{service: s}
}

// ChangeAllowed checks if change block is in place.
// Change block prevents all operations that may change
// current model in any way from running successfully.
func (c *BlockChecker) ChangeAllowed(ctx context.Context) error {
	return c.checkBlock(ctx, blockcommand.ChangeBlock)
}

// RemoveAllowed checks if remove block is in place.
// Remove block prevents removal of machine, service, unit
// and relation from current model.
func (c *BlockChecker) RemoveAllowed(ctx context.Context) error {
	if err := c.checkBlock(ctx, blockcommand.RemoveBlock); err != nil {
		return err
	}
	// Check if change block has been enabled
	return c.checkBlock(ctx, blockcommand.ChangeBlock)
}

// DestroyAllowed checks if destroy block is in place.
// Destroy block prevents destruction of current model.
func (c *BlockChecker) DestroyAllowed(ctx context.Context) error {
	if err := c.checkBlock(ctx, blockcommand.DestroyBlock); err != nil {
		return err
	}
	// Check if remove block has been enabled
	if err := c.checkBlock(ctx, blockcommand.RemoveBlock); err != nil {
		return err
	}
	// Check if change block has been enabled
	return c.checkBlock(ctx, blockcommand.ChangeBlock)
}

// checkBlock checks if specified operation must be blocked.
// If it does, the method throws specific error that can be examined
// to stop operation execution.
func (c *BlockChecker) checkBlock(ctx context.Context, blockType blockcommand.BlockType) error {
	message, err := c.service.GetBlockSwitchedOn(ctx, blockType)
	if errors.Is(err, blockcommanderrors.NotFound) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return apiservererrors.OperationBlockedError(message)
}
