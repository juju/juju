// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/internal/errors"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetBlock switches on a command block for a given type with an optional
	// message.
	SetBlock(ctx context.Context, t blockcommand.BlockType, message string) error

	// RemoveBlock disables block of specified type for the current model.
	RemoveBlock(ctx context.Context, t blockcommand.BlockType) error

	// RemoveAllBlocks removes all the blocks for the current model.
	RemoveAllBlocks(ctx context.Context) error

	// GetBlocks returns all the blocks for the current model.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)

	// GetBlockMessage returns the optional block message if it is switched on.
	GetBlockMessage(ctx context.Context, t blockcommand.BlockType) (string, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// GetBlockSwitchedOn returns the optional block message if it is switched on
// for the given type.
// Returns an error [errors.NotFound] if the block does not exist.
func (s *Service) GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}

	return s.st.GetBlockMessage(ctx, t)
}

// SwitchBlockOn switches on a command block for a given type and message.
// Returns an error [errors.AlreadyExists] if the block already exists.
func (s *Service) SwitchBlockOn(ctx context.Context, t blockcommand.BlockType, message string) error {
	if err := t.Validate(); err != nil {
		return err
	}

	if len(message) > blockcommand.DefaultMaxMessageLength {
		return errors.Errorf("message length exceeds maximum allowed length of %d", blockcommand.DefaultMaxMessageLength)
	}

	if err := s.st.SetBlock(ctx, t, message); errors.Is(err, blockcommanderrors.AlreadyExists) {
		s.logger.Debugf(ctx, "block already exists for type %q", t)
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

// GetBlocks returns all the blocks for the current model.
func (s *Service) GetBlocks(ctx context.Context) ([]blockcommand.Block, error) {
	return s.st.GetBlocks(ctx)
}

// SwitchBlockOff disables block of specified type for the current model.
// Returns an error [errors.NotFound] if the block does not exist.
func (s *Service) SwitchBlockOff(ctx context.Context, t blockcommand.BlockType) error {
	if err := t.Validate(); err != nil {
		return err
	}

	return s.st.RemoveBlock(ctx, t)
}

// RemoveAllBlocks removes all the blocks for the current model.
func (s *Service) RemoveAllBlocks(ctx context.Context) error {
	return s.st.RemoveAllBlocks(ctx)
}
