// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corestatus "github.com/juju/juju/core/status"
)

// ApplicationStatusService sets the status of unitless applications.
type ApplicationStatusService interface {
	// SetApplicationStatusByUUID sets the given application's status.
	SetApplicationStatusByUUID(
		ctx context.Context,
		applicationUUID coreapplication.UUID,
		statusInfo corestatus.StatusInfo,
	) error
}
