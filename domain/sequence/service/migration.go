// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import "context"

// State describes retrieval and persistence methods for sequences.
type State interface {
	// GetSequencesForExport returns the sequences for export. This is used to
	// retrieve the sequences for export in the database.
	GetSequencesForExport(ctx context.Context) (map[string]uint64, error)

	// ImportSequences imports the sequences from the given map. This is used to
	// import the sequences from the database.
	ImportSequences(ctx context.Context, seqs map[string]uint64) error

	// RemoveAllSequences removes all sequences from the database. This is used
	// to remove all sequences from the database.
	RemoveAllSequences(ctx context.Context) error
}

// MigrationServic provides the API for working with sequences.
type MigrationServic struct {
	st State
}

// NewMigrationService creates a new migration service for the given state.
func NewMigrationService(st State) *MigrationServic {
	return &MigrationServic{
		st: st,
	}
}

// GetSequencesForExport returns the sequences for export. This is used to
// retrieve the sequences for export in the database.
func (m *MigrationServic) GetSequencesForExport(ctx context.Context) (map[string]uint64, error) {
	return m.st.GetSequencesForExport(ctx)
}

// ImportSequences imports the sequences from the given map. This is used to
// import the sequences from the database.
func (m *MigrationServic) ImportSequences(ctx context.Context, seqs map[string]uint64) error {
	return m.st.ImportSequences(ctx, seqs)
}

// RemoveAllSequences removes all sequences from the database. This is used to
// remove all sequences from the database.
func (m *MigrationServic) RemoveAllSequences(ctx context.Context) error {
	return m.st.RemoveAllSequences(ctx)
}
