// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corestatus "github.com/juju/juju/core/status"
)

// ApplicationStatusService sets the status of scriptlet applications.
type ApplicationStatusService interface {
	// SetApplicationStatusByID sets the status of the application with the
	// given UUID, overwriting any current status data.
	SetApplicationStatusByID(
		ctx context.Context,
		applicationUUID coreapplication.UUID,
		statusInfo corestatus.StatusInfo,
	) error
}
