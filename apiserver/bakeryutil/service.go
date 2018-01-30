// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakeryutil

import (
	"time"

	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/bakerystorage"
)

// BakeryServicePublicKeyLocator is an implementation of
// bakery.PublicKeyLocator that simply returns the embedded
// bakery service's public key.
type BakeryServicePublicKeyLocator struct {
	Service *bakery.Service
}

// PublicKeyForLocation implements bakery.PublicKeyLocator.
func (b BakeryServicePublicKeyLocator) PublicKeyForLocation(string) (*bakery.PublicKey, error) {
	return b.Service.PublicKey(), nil
}

// NewBakeryService returns a new bakery.Service and bakery.KeyPair.
// The bakery service is identifeid by the model corresponding to the
// State.
func NewBakeryService(
	st *state.State,
	store bakerystorage.ExpirableStorage,
	locator bakery.PublicKeyLocator,
) (*bakery.Service, *bakery.KeyPair, error) {
	key, err := bakery.GenerateKey()
	if err != nil {
		return nil, nil, errors.Annotate(err, "generating key for bakery service")
	}
	service, err := bakery.NewService(bakery.NewServiceParams{
		Location: "juju model " + st.ModelUUID(),
		Store:    store,
		Key:      key,
		Locator:  locator,
	})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return service, key, nil
}

// ExpirableStorageBakeryService wraps bakery.Service,
// adding the ExpireStorageAt method.
type ExpirableStorageBakeryService struct {
	*bakery.Service
	Key     *bakery.KeyPair
	Store   bakerystorage.ExpirableStorage
	Locator bakery.PublicKeyLocator
}

// ExpireStorageAt implements authentication.ExpirableStorageBakeryService.
func (s *ExpirableStorageBakeryService) ExpireStorageAt(t time.Time) (authentication.ExpirableStorageBakeryService, error) {
	store := s.Store.ExpireAt(t)
	service, err := bakery.NewService(bakery.NewServiceParams{
		Location: s.Location(),
		Store:    store,
		Key:      s.Key,
		Locator:  s.Locator,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ExpirableStorageBakeryService{service, s.Key, store, s.Locator}, nil
}
