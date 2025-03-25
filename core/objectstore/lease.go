// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

const (
	// ObjectStoreLeaseHolderName is the name of the lease holder for the
	// object store.
	ObjectStoreLeaseHolderName = "objectstore"
)

// ParseLeaseHolderName returns true if the supplied name is a valid lease
// holder.
// This is used to ensure that the lease manager does not attempt to acquire
// leases for invalid names.
func ParseLeaseHolderName(name string) error {
	if name == ObjectStoreLeaseHolderName {
		return nil
	}
	return errors.Errorf("lease holder name %q %w", name, coreerrors.NotValid)
}
