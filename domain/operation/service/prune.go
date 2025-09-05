// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"
)

// PruneOperations removes operations older than maxAge or larger than maxSizeMB.
func (s *Service) PruneOperations(ctx context.Context, maxAge time.Duration, maxSizeMB int) error {
	s.logger.Infof(ctx, "PruneOperations: not implemented (maxAge=%s maxSizeMB=%d)", maxAge, maxSizeMB)
	return nil
}
