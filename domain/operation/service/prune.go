// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// PruneOperations removes operations older than maxAge or larger than maxSizeMB
// (in megabytes).
func (s *Service) PruneOperations(ctx context.Context, maxAge time.Duration, maxSizeMB int) error {
	if maxAge < 0 || maxSizeMB < 0 {
		return errors.Errorf("max age and size should be positive (maxAge=%s maxSizeMB=%d)", maxAge,
			maxSizeMB).Add(coreerrors.NotValid)
	}

	// todo(gfouillet): In a followup PR, we should prune the freed data from the object store.
	//   this will be done by returning the storeUUID freed by the state prune operation.
	//   and calling another state method to prune them specifically.
	return errors.Capture(s.st.PruneOperations(ctx, maxAge, maxSizeMB))
}
