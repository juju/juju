// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"sync"

	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// Service provides controller-scoped SSH host key workflows.
type Service struct {
	state State

	// publicKeyOnce guards the one-time derivation of the marshalled public
	// host key. The controller SSH host key is set once at bootstrap and is not
	// rotated, so deriving the public key from it is safe to cache for the
	// lifetime of the service.
	publicKeyOnce sync.Once
	publicKey     []byte
	publicKeyErr  error
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
// controller SSH jump server. It derives the public key from the stored private
// host key and caches the result, so the private key is fetched from state and
// parsed only once rather than on every call. The controller SSH host key is
// set once at bootstrap and is not rotated, so caching for the lifetime of the
// service is safe.
func (s *Service) SSHServerHostPublicKey(ctx context.Context) ([]byte, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	s.publicKeyOnce.Do(func() {
		privateHostKey, err := s.SSHServerHostKey(ctx)
		if err != nil {
			s.publicKeyErr = errors.Capture(err)
			return
		}
		signer, err := gossh.ParsePrivateKey([]byte(privateHostKey))
		if err != nil {
			s.publicKeyErr = errors.Errorf("parsing controller SSH host key: %w", err)
			return
		}
		s.publicKey = signer.PublicKey().Marshal()
	})
	if s.publicKeyErr != nil {
		return nil, s.publicKeyErr
	}
	return s.publicKey, nil
}
