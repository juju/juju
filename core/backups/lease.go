// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// ParseLeaseHolderName validates a controller node ID used as a lease holder.
// This is used to ensure that the lease manager does not attempt to acquire
// leases for invalid names.
func ParseLeaseHolderName(name string) error {
	if name != "" {
		return nil
	}
	return errors.Errorf("lease holder name %q %w", name, coreerrors.NotValid)
}
