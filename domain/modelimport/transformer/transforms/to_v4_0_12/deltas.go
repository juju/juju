// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_0_12

import (
	"context"

	"github.com/juju/juju/domain/export/types/v4_0_11"
	"github.com/juju/juju/domain/export/types/v4_0_12"
)

// deltas is the engineer-owned implementation of the Deltas interface
// declared in transform.go. When Deltas has methods, add receivers on
// this type or the package will not compile.
type deltas struct{}

var _ Deltas = deltas{}

// NewDeltas returns the engineer-written delta implementation for the
// 4.0.11 -> 4.0.12 transform.
func NewDeltas() Deltas { return deltas{} }

// SecretReservation returns no rows for 4.0.11 payloads. The source schema
// has no secret reservation table.
func (d deltas) SecretReservation(_ context.Context, _ *v4_0_11.ModelExport) ([]v4_0_12.SecretReservation, error) {
	return nil, nil
}
