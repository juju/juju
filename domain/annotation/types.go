// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotation

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// GetCharmArgs holds the arguments for the GetCharm method.
type GetCharmArgs struct {
	Source   string
	Name     string
	Revision int
}

// Validate checks if the GetCharmArgs are valid or not.
func (a GetCharmArgs) Validate() error {
	if a.Source == "" {
		return errors.Errorf("source %w", coreerrors.NotValid)
	}
	if a.Name == "" {
		return errors.Errorf("name %w", coreerrors.NotValid)
	}
	if a.Revision < 0 {
		return errors.Errorf("negative revision %w", coreerrors.NotValid)
	}
	return nil
}
