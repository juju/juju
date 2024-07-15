// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/errors"

	"github.com/juju/juju/domain/macaroon"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
)

// RootKeyState describes the persistence layer for
// macaroon root keys
type RootKeyState interface {
	// GetKey gets the key with a given id from state. If not key is found, a
	// macaroonerrors.KeyNotFound error is returned.
	GetKey(ctx context.Context, id []byte) (macaroon.RootKey, error)

	// FindLatestKey returns the most recently created root key k following all
	// the conditions:
	//
	// k.Created >= createdAfter
	// k.Expires >= expiresAfter
	// k.Expires <= expiresBefore
	//
	// If no such key was found, return a macaroonerrors.KeyNotFound error
	FindLatestKey(ctx context.Context, createdAfter, expiresAfter, expiresBefore time.Time) (macaroon.RootKey, error)

	// InsertKey inserts the given root key into state. If a key with matching
	// id already exists, return a macaroonerrors.KeyAlreadyExists error.
	InsertKey(ctx context.Context, key macaroon.RootKey) error
}

// RootKeyService provides the API for macaroon root key storage
//
// RootKeyService satisfies dbrootkeystore.Backing and dbrootkeystore.ContextBacking
// https://github.com/go-macaroon-bakery/macaroon-bakery/blob/f9b21e15a2ed91756aa172972c7178992c7fe6d1/bakery/dbrootkeystore/rootkey.go#L48-L95
//
// This means RootKeyService can be used to construct a bakery.RootKeyStore.
//
// NOTE: We implement dbrootkeystore.Backing with stub methods. This is because
// RootKeyStore only requires a ContextBacking, but due to pecularities with
// the RootKeyStore constructor, we also need to implement Backing.
// TODO(jack-w-shaw): Once https://github.com/go-macaroon-bakery/macaroon-bakery/pull/301
// has been released, use NewContextStore & drop Backing stub methods
type RootKeyService struct {
	st RootKeyState
}

// NewRootKeyService returns a new service for managing macaroon root keys
func NewRootKeyService(st RootKeyState) *RootKeyService {
	return &RootKeyService{
		st: st,
	}
}

// GetKey is a stub method required to implement dbrootstore.Backing. Do not use
func (s *RootKeyService) GetKey(id []byte) (dbrootkeystore.RootKey, error) {
	return dbrootkeystore.RootKey{}, errors.NotImplementedf("GetKey")
}

// GetKeyContext (dbrootkeystore.GetKeyContext) gets the key
// with a given id from dqlite.
//
// To satisfy dbrootkeystore.ContextBacking specification,
// if not key is found, a bakery.ErrNotFound error is returned.
func (s *RootKeyService) GetKeyContext(ctx context.Context, id []byte) (dbrootkeystore.RootKey, error) {
	key, err := s.st.GetKey(ctx, id)
	if errors.Is(err, macaroonerrors.KeyNotFound) {
		return dbrootkeystore.RootKey{}, bakery.ErrNotFound
	}
	return decodeRootKey(key), nil
}

// FindLatestKey is a stub method required to implement dbrootstore.Backing. Do not use
func (s *RootKeyService) FindLatestKey(createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	return dbrootkeystore.RootKey{}, errors.NotImplementedf("FindLatestKey")
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
func (s *RootKeyService) FindLatestKeyContext(ctx context.Context, createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	key, err := s.st.FindLatestKey(ctx, createdAfter, expiresAfter, expiresBefore)
	if errors.Is(err, macaroonerrors.KeyNotFound) {
		return dbrootkeystore.RootKey{}, nil
	}
	return decodeRootKey(key), err
}

// InsertKey is a stub method required to implement dbrootstore.Backing. Do not use
func (s *RootKeyService) InsertKey(key dbrootkeystore.RootKey) error {
	return errors.NotImplementedf("InsertKey")
}

// InsertKeyContext (dbrootkeystore.InsertKeyContext) inserts
// the given root key into state. If a key with matching
// id already exists, return a macaroonerrors.KeyAlreadyExists error.
func (s *RootKeyService) InsertKeyContext(ctx context.Context, key dbrootkeystore.RootKey) error {
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
