// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/ssh"
)

// ControllerKeyService provides a service for interacting the the underlying
// ssh keys stored for a controller.
type ControllerKeyService struct {
	st ControllerKeyState
}

// ControllerKeyState defines the state layer for interacting with a controllers
// config and ssh keys.
type ControllerKeyState interface {
	GetControllerConfigKeys(context.Context, []string) (map[string]string, error)
}

// NewControllerKeyService constructs a new [ControllerKeyService] for
// retrieving the controllers ssh keys.
func NewControllerKeyService(st ControllerKeyState) *ControllerKeyService {
	return &ControllerKeyService{
		st: st,
	}
}

// ControllerKeys returns the juju controllers ssh public keys. If no keys are
// defined for the controller a zero length slice is returned.
func (s *ControllerKeyService) ControllerKeys(ctx context.Context) ([]string, error) {
	ctrlConfig, err := s.st.GetControllerConfigKeys(ctx, []string{controller.SystemSSHKeys})
	if err != nil {
		return nil, fmt.Errorf("cannot get juju controller public ssh keys: %w", err)
	}

	return ssh.SplitAuthorisedKeys(ctrlConfig[controller.SystemSSHKeys]), nil
}
