// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// Service provides controller-scoped SSH host key workflows.
type Service struct {
	state State
}

// NewService returns a new controller SSH service.
func NewService(state State) *Service {
	return &Service{state: state}
}

// SSHServerHostKey returns the controller jump host key.
func (s *Service) SSHServerHostKey(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, found, err := s.state.GetSSHServerHostKey(ctx)
	if err != nil {
		return "", errors.Errorf("getting controller SSH server host key: %w", err)
	}
	if !found {
		return "", errors.Errorf("controller SSH server host key not found")
	}
	return key, nil
}
