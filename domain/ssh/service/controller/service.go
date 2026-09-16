// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	gossh "golang.org/x/crypto/ssh"

	coremodel "github.com/juju/juju/core/model"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
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

	key, err := s.state.GetSSHServerHostKey(ctx)
	if err != nil {
		return "", errors.Errorf("getting controller SSH server host key: %w", err)
	}
	return key, nil
}

// SSHServerHostPublicKey returns the marshalled public host key of the
// controller SSH jump server. The public key is derived once at bootstrap and
// stored alongside the private key, so this method simply reads the stored
// value and never handles private key material.
func (s *Service) SSHServerHostPublicKey(ctx context.Context) ([]byte, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, err := s.state.GetSSHServerHostPublicKey(ctx)
	if err != nil {
		return nil, errors.Errorf("getting controller SSH server host public key: %w", err)
	}
	return key, nil
}

// GetPublicKeysForUser returns all public SSH keys registered for a user.
func (s *Service) GetPublicKeysForUser(ctx context.Context, username user.Name) ([]coressh.PublicKey, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	keys, err := s.state.GetPublicKeysForUser(ctx, username)
	if err != nil {
		return nil, errors.Errorf("getting public SSH keys for user %q: %w", username, err)
	}
	return keys, nil
}

// PublicKeyInModel reports whether key is authorized for username in modelUUID.
func (s *Service) PublicKeyInModel(ctx context.Context, modelUUID coremodel.UUID, username user.Name, key gossh.PublicKey) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return false, errors.Errorf("validating model UUID %q: %w", modelUUID, err)
	}

	keys, err := s.state.GetPublicKeysForUserInModel(ctx, modelUUID.String(), username.Name())
	if err != nil {
		return false, errors.Errorf("getting public SSH keys for user %q: %w", username, err)
	}
	for _, modelKey := range keys {
		if modelKey.Fingerprint == gossh.FingerprintSHA256(key) {
			return true, nil
		}
	}
	return false, nil
}
