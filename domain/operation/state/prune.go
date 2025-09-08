// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"
)

// PruneOperations deletes operations older than maxAge and larger than maxSizeMB.
func (s State) PruneOperations(ctx context.Context, maxAge time.Duration, maxSizeMB int) error {
	s.logger.Infof(ctx, "PruneOperations: not implemented (maxAge=%s maxSizeMB=%d)", maxAge, maxSizeMB)
	return nil
}
