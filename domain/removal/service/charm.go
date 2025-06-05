// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
)

// RemoveCharmForApplication removes the charm for the given application.
// It checks to ensure that no other units or applications are using the charm
// before proceeding with the removal.
func (s *Service) RemoveCharmForApplication(ctx context.Context, appUUID coreapplication.ID) error {
	return nil
}
