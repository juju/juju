// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// GetModelCloudType returns the type of the cloud that is in use by this model.
func (s *Service) GetModelCloudType(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	cloudType, err := s.st.GetModelCloudType(ctx)
	return cloudType, errors.Capture(err)
}
