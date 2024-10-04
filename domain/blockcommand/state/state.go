// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents database interactions dealing with command block.
type State struct {
	*domain.StateBase
}

// NewState returns a new block device state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetBlock switches on a command block for a given type with an optional
// message.
// Returns an error [errors.BlockAlreadyExists].
func (s *State) SetBlock(ctx context.Context, t blockcommand.BlockType, message string) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	bcType, err := encodeBlockType(t)
	if err != nil {
		return err
	}

	bc := blockCommand{
		BlockType: bcType,
		Message:   message,
	}

	stmt, err := s.Prepare("INSERT INTO block_command (*) VALUES ($blockCommand.*)", bc)
	if err != nil {
		return errors.Errorf("preparing block command statement: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, stmt, bc).Get(&outcome); database.IsErrConstraintPrimaryKey(err) {
			return blockcommanderrors.AlreadyExists
		} else if err != nil {
			return errors.Errorf("inserting block command: %w", err)
		}

		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("getting rows affected: %w", err)
		} else if affected != 1 {
			return errors.Errorf("expected 1 row affected, got %d", affected)
		}

		return nil
	}); err != nil {
		return errors.Errorf("executing block command: %w", err)
	}

	return nil
}

// RemoveBlock disables block of specified type for the current model.
// Returns an error [errors.BlockNotFound].
func (s *State) RemoveBlock(ctx context.Context, t blockcommand.BlockType) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	bcType, err := encodeBlockType(t)
	if err != nil {
		return err
	}

	bc := blockType{ID: bcType}

	stmt, err := s.Prepare("DELETE FROM block_command WHERE block_command_type_id = $blockType.id", bc)
	if err != nil {
		return errors.Errorf("preparing block command statement: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, stmt, bc).Get(&outcome); err != nil {
			return errors.Errorf("deleting block command: %w", err)
		}

		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("getting rows affected: %w", err)
		} else if affected == 0 {
			return blockcommanderrors.NotFound
		}

		return nil
	}); err != nil {
		return errors.Errorf("executing block command: %w", err)
	}

	return nil
}

// GetBlocks returns all the blocks for the current model.
func (s *State) GetBlocks(ctx context.Context) ([]blockcommand.Block, error) {
	db, err := s.DB()
	if err != nil {
		return nil, err
	}

	var block blockCommand
	stmt, err := s.Prepare("SELECT &blockCommand.* FROM block_command ORDER BY rowid", block)
	if err != nil {
		return nil, errors.Errorf("preparing block command statement: %w", err)
	}

	var blocks []blockCommand
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&blocks); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting block commands: %w", err)
		}

		return nil
	}); err != nil {
		return nil, errors.Errorf("executing block command: %w", err)
	}

	var results []blockcommand.Block
	for _, b := range blocks {
		bt, err := decodeBlockType(b.BlockType)
		if err != nil {
			return nil, err
		}

		results = append(results, blockcommand.Block{
			Type:    bt,
			Message: b.Message,
		})
	}

	return results, nil
}

func encodeBlockType(t blockcommand.BlockType) (int8, error) {
	switch t {
	case blockcommand.DestroyBlock, blockcommand.RemoveBlock, blockcommand.ChangeBlock:
		return int8(t) - 1, nil
	}
	return 0, errors.Errorf("invalid block type %d", t)
}

func decodeBlockType(t int8) (blockcommand.BlockType, error) {
	bt := blockcommand.BlockType(t + 1)
	if err := bt.Validate(); err != nil {
		return -1, err
	}
	return bt, nil
}
