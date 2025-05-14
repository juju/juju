// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// BakeryConfigState describes persistence layer methods for bakery config
type BakeryConfigState interface {
	// InitialiseBakeryConfig creates and fills in the bakery config in state.
	InitialiseBakeryConfig(ctx context.Context, localUsersKey, localUsersThirdPartyKey, externalUsersThirdPartyKey, offersThirdPartyKey *bakery.KeyPair) error

	// GetLocalUsersKey returns the key pair used with the local users bakery.
	GetLocalUsersKey(context.Context) (*bakery.KeyPair, error)

	// GetLocalUsersThirdPartyKey returns the third party key pair used with the local users bakery.
	GetLocalUsersThirdPartyKey(context.Context) (*bakery.KeyPair, error)

	// GetExternalUsersThirdPartyKey returns the third party key pair used with the external users bakery.
	GetExternalUsersThirdPartyKey(context.Context) (*bakery.KeyPair, error)

	// GetOffersThirdPartyKey returns the key pair used with the cross model offers bakery.
	GetOffersThirdPartyKey(context.Context) (*bakery.KeyPair, error)
}

// BakeryConfigService provides the API for the bakery config
type BakeryConfigService struct {
	st BakeryConfigState
}

// NewBakeryConfigService returns a new service for managing bakery config
func NewBakeryConfigService(st BakeryConfigState) *BakeryConfigService {
	return &BakeryConfigService{
		st: st,
	}
}

// InitialiseBakeryConfig creates and fills in the bakery config in state.
func (s *BakeryConfigService) InitialiseBakeryConfig(ctx context.Context) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	localUsersKey, err := bakery.GenerateKey()
	if err != nil {
		return errors.Errorf("generating local users keypair: %w", err)
	}

	localUsersThirdPartyKey, err := bakery.GenerateKey()
	if err != nil {
		return errors.Errorf("generating local users third party keypair: %w", err)
	}

	externalUsersThirdPartyKey, err := bakery.GenerateKey()
	if err != nil {
		return errors.Errorf("generating external users third party keypair: %w", err)
	}

	offersThirdPartyKey, err := bakery.GenerateKey()
	if err != nil {
		return errors.Errorf("generating offers third party keypair: %w", err)
	}

	err = s.st.InitialiseBakeryConfig(
		ctx,
		localUsersKey,
		localUsersThirdPartyKey,
		externalUsersThirdPartyKey,
		offersThirdPartyKey,
	)
	if err != nil {
		return errors.Errorf("initialising bakery config: %w", err)
	}
	return nil
}

// GetLocalUsersKey returns the key pair used with the local users bakery.
func (s *BakeryConfigService) GetLocalUsersKey(ctx context.Context) (_ *bakery.KeyPair, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	keyPair, err := s.st.GetLocalUsersKey(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return keyPair, nil
}

// GetLocalUsersThirdPartyKey returns the third party key pair used with the local users bakery.
func (s *BakeryConfigService) GetLocalUsersThirdPartyKey(ctx context.Context) (_ *bakery.KeyPair, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	keyPair, err := s.st.GetLocalUsersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return keyPair, nil
}

// GetExternalUsersThirdPartyKey returns the third party key pair used with the external users bakery.
func (s *BakeryConfigService) GetExternalUsersThirdPartyKey(ctx context.Context) (_ *bakery.KeyPair, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	keyPair, err := s.st.GetExternalUsersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return keyPair, nil
}

// GetOffersThirdPartyKey returns the key pair used with the cross model offers bakery.
func (s *BakeryConfigService) GetOffersThirdPartyKey(ctx context.Context) (_ *bakery.KeyPair, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	keyPair, err := s.st.GetOffersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return keyPair, nil
}
