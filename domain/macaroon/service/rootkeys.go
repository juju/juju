// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/macaroon"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/internal/errors"
)

// RootKeyState describes the persistence layer for
// macaroon root keys
type RootKeyState interface {
	// GetKey gets the key with a given id from state. If not key is found, a
	// macaroonerrors.KeyNotFound error is returned.
	GetKey(ctx context.Context, id []byte, now time.Time) (macaroon.RootKey, error)

	// FindLatestKey returns the most recently created root key k following all
	// the conditions:
	//
	// k.Created >= createdAfter
	// k.Expires >= expiresAfter
	// k.Expires <= expiresBefore
	//
	// If no such key was found, return a macaroonerrors.KeyNotFound error
	FindLatestKey(ctx context.Context, createdAfter, expiresAfter, expiresBefore, now time.Time) (macaroon.RootKey, error)

	// InsertKey inserts the given root key into state. If a key with matching
	// id already exists, return a macaroonerrors.KeyAlreadyExists error.
	InsertKey(ctx context.Context, key macaroon.RootKey) error
}

// RootKeyService provides the API for macaroon root key storage
// We can use RootKeyService to construct a bakery.RootKeyStore.
type RootKeyService struct {
	clock macaroon.Clock
	st    RootKeyState
}

// NewRootKeyService returns a new service for managing macaroon root keys
func NewRootKeyService(st RootKeyState, clock macaroon.Clock) *RootKeyService {
	return &RootKeyService{
		st:    st,
		clock: clock,
	}
}

// GetKeyContext (dbrootkeystore.GetKeyContext) gets the key
// with a given id from dqlite.
//
// To satisfy dbrootkeystore.ContextBacking specification,
// if not key is found, a bakery.ErrNotFound error is returned.
func (s *RootKeyService) GetKeyContext(ctx context.Context, id []byte) (_ dbrootkeystore.RootKey, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, err := s.st.GetKey(ctx, id, s.clock.Now())
	if errors.Is(err, macaroonerrors.KeyNotFound) {
		return dbrootkeystore.RootKey{}, bakery.ErrNotFound
	}
	return decodeRootKey(key), nil
}

// FindLatestKeyContext (dbrootkeystore.FindLatestKeyContext) returns
// the most recently created root key k following all
// the conditions:
//
// k.Created >= createdAfter
// k.Expires >= expiresAfter
// k.Expires <= expiresBefore
//
// To satisfy dbrootkeystore.FindLatestKeyContext specification,
// if no such key is found, the zero root key is returned with a
// nil error
func (s *RootKeyService) FindLatestKeyContext(ctx context.Context, createdAfter, expiresAfter, expiresBefore time.Time) (_ dbrootkeystore.RootKey, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, err := s.st.FindLatestKey(ctx, createdAfter, expiresAfter, expiresBefore, s.clock.Now())
	if errors.Is(err, macaroonerrors.KeyNotFound) {
		return dbrootkeystore.RootKey{}, nil
	}
	return decodeRootKey(key), err
}

// InsertKeyContext (dbrootkeystore.InsertKeyContext) inserts
// the given root key into state. If a key with matching
// id already exists, return a macaroonerrors.KeyAlreadyExists error.
func (s *RootKeyService) InsertKeyContext(ctx context.Context, key dbrootkeystore.RootKey) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.InsertKey(ctx, encodeRootKey(key))
}

func encodeRootKey(k dbrootkeystore.RootKey) macaroon.RootKey {
	return macaroon.RootKey{
		ID:      k.Id,
		Created: k.Created,
		Expires: k.Expires,
		RootKey: k.RootKey,
	}
}

func decodeRootKey(k macaroon.RootKey) dbrootkeystore.RootKey {
	return dbrootkeystore.RootKey{
		Id:      k.ID,
		Created: k.Created,
		Expires: k.Expires,
		RootKey: k.RootKey,
	}
}
