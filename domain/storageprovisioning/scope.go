// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

// OwnershipScope declares to the model in what context a storage entity needs
// to be owned.
type OwnershipScope int

const (
	// OwnershipScopeModel indicates that the ownership for the storage instance
	// is to the model.
	OwnershipScopeModel OwnershipScope = iota

	// OwnershipScopeMachine indicates that the ownership for the storage
	// instance is to be the machine.
	OwnershipScopeMachine
)
