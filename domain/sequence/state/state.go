// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	sequenceerrors "github.com/juju/juju/domain/sequence/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State provides persistence and retrieval associated with entity removal.
type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetSequencesForExport returns the sequences for export. This is used to
// retrieve the sequences for export in the database.
func (s *State) GetSequencesForExport(ctx context.Context) (map[string]uint64, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, err
	}

	query, err := s.Prepare("SELECT &sequence.* FROM sequence", sequence{})
	if err != nil {
		return nil, err
	}

	var sequences []sequence
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, query).GetAll(&sequences); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	seqs := make(map[string]uint64)
	for _, seq := range sequences {
		seqs[seq.Namespace] = seq.Value
	}
	return seqs, nil
}

// ImportSequences imports the sequences from the given map. This is used to
// import the sequences from the database.
func (s *State) ImportSequences(ctx context.Context, seqs map[string]uint64) error {
	db, err := s.DB(ctx)
	if err != nil {
		return err
	}

	query, err := s.Prepare("INSERT INTO sequence (*) VALUES ($sequence.*)", sequence{})
	if err != nil {
		return err
	}

	sequences := make([]sequence, 0, len(seqs))
	for namespace, value := range seqs {
		sequences = append(sequences, sequence{
			Namespace: namespace,
			Value:     value,
		})
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, sequences).Run()
		if internaldatabase.IsErrConstraintPrimaryKey(err) {
			return sequenceerrors.DuplicateNamespaceSequence
		} else if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// RemoveAllSequences removes all sequences from the database.
func (s *State) RemoveAllSequences(ctx context.Context) error {
	db, err := s.DB(ctx)
	if err != nil {
		return err
	}

	query, err := s.Prepare("DELETE FROM sequence")
	if err != nil {
		return err
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, query).Run(); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
