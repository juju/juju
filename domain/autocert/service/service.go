// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"golang.org/x/crypto/acme/autocert"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	// Put implements autocert.Cache.Put.
	Put(ctx context.Context, name string, data []byte) error

	// Get implements autocert.Cache.Get.
	Get(ctx context.Context, name string) ([]byte, error)

	// Delete implements autocert.Cache.Delete.
	Delete(ctx context.Context, name string) error
}

// Service provides the API for working with autocert cache. This service
// implements autocert.Cache interface.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// Put implements autocert.Cache.Put.
func (s *Service) Put(ctx context.Context, name string, data []byte) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	s.logger.Tracef(ctx, "storing autocert %s with contents '%+v' in the autocert cache", name, string(data))
	return s.st.Put(ctx, name, data)
}

// Get implements autocert.Cache.Get.
func (s *Service) Get(ctx context.Context, name string) (_ []byte, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	s.logger.Tracef(ctx, "retrieving autocert %s from the autocert cache", name)
	cert, err := s.st.Get(ctx, name)
	if errors.Is(err, coreerrors.NotFound) {
		return nil, autocert.ErrCacheMiss
	}
	return cert, err
}

// Delete implements autocert.Cache.Delete.
func (s *Service) Delete(ctx context.Context, name string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	s.logger.Tracef(ctx, "removing autocert %s from the autocert cache", name)
	return s.st.Delete(ctx, name)
}
