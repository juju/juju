// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/ssh"
)

// ControllerKeyService provides a service for interacting with the underlying
// ssh keys stored for a controller.
type ControllerKeyService struct {
	st ControllerKeyState
}

// ControllerKeyState defines the state layer for interacting with a
// controller's config and ssh keys.
type ControllerKeyState interface {
	GetControllerConfigKeys(context.Context, []string) (map[string]string, error)
}

// NewControllerKeyService constructs a new [ControllerKeyService] for
// retrieving the controller's ssh keys.
func NewControllerKeyService(st ControllerKeyState) *ControllerKeyService {
	return &ControllerKeyService{
		st: st,
	}
}

// ControllerAuthorisedKeys returns the juju controller's ssh public keys. If no
// keys are defined for the controller, a zero length slice is returned.
func (s *ControllerKeyService) ControllerAuthorisedKeys(
	ctx context.Context,
) (_ []string, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	ctrlConfig, err := s.st.GetControllerConfigKeys(ctx, []string{controller.SystemSSHKeys})
	if err != nil {
		return nil, errors.Errorf("cannot get juju controller public ssh keys: %w", err)
	}

	keys, err := ssh.SplitAuthorizedKeys(ctrlConfig[controller.SystemSSHKeys])
	if err != nil {
		return nil, errors.Errorf(
			"cannot split authorized keys from controller config system ssh keys: %w",
			err)

	}

	return keys, nil
}
