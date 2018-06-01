// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
)

// InvalidateModelCredential invalidate cloud credential for the model
// of the given state.
func (st *State) InvalidateModelCredential(reason string) error {
	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	tag, exists := m.CloudCredential()
	if !exists {
		// Model is on the cloud that does not require auth - nothing to do.
		return nil
	}

	return errors.Trace(st.InvalidateCloudCredential(tag, reason))
}
