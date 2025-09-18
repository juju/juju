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

	storePaths, err := s.st.PruneOperations(ctx, maxAge, maxSizeMB)
	if err != nil {
		return errors.Capture(err)
	}
	if len(storePaths) == 0 {
		return nil
	}

	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	var errs []error
	for _, path := range storePaths {
		// We accumulate errors to allow a maximum of remove, even if we get
		// some errors.
		errs = append(errs, objectStore.Remove(ctx, path))
	}
	return errors.Capture(errors.Join(errs...))
}
