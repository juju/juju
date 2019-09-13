// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// OpFactory describes a source of model operations
// required by the model generation API.
type OpFactory interface {
	// NewCommitBranchModelOp returns an operation for committing a branch.
	NewCommitBranchModelOp(branchName, userName string) (CommitBranchModelOp, error)
}

type opFactory struct {
	st *state.State
}

func newOpFactory(st *state.State) OpFactory {
	return &opFactory{st: st}
}

// NewCommitBranchModelOp (OpFactory) returns an operation
// for committing a branch.
func (f *opFactory) NewCommitBranchModelOp(branchName, userName string) (CommitBranchModelOp, error) {
	br, err := f.st.Branch(branchName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewCommitBranchModelOp(commitBranchStateShim{f.st}, br, userName, state.NewStateSettings(f.st)), nil
}
