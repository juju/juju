// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// IAASModel contains data that is specific to an
// Infrastructure-As-A-Service (IAAS) model.
type IAASModel struct {
	mb modelBackend

	// TODO(jsing): This should be removed once things
	// have been sufficiently untangled.
	st *State
}
