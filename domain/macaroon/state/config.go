// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	macroonerrors "github.com/juju/juju/domain/macaroon/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// BakeryConfigState describes the persistence layer for
// the macaroon bakery config
type BakeryConfigState struct {
	*domain.StateBase
}

// NewBakeryConfigState returns a new config state reference
func NewBakeryConfigState(factory coredatabase.TxnRunnerFactory) *BakeryConfigState {
	return &BakeryConfigState{
		StateBase: domain.NewStateBase(factory),
	}
}

// InitialiseBakeryConfig creates and fills in the bakery config in state.
func (st *BakeryConfigState) InitialiseBakeryConfig(
	ctx context.Context,
	localUsersKey,
	localUsersThirdPartyKey,
	externalUsersThirdPartyKey,
	offersThirdPartyKey *bakery.KeyPair,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	cfg := FullConfig{
		LocalUsersPrivateKey:              &keyScanner{key: localUsersKey.Private.Key},
		LocalUsersPublicKey:               &keyScanner{key: localUsersKey.Public.Key},
		LocalUsersThirdPartyPrivateKey:    &keyScanner{key: localUsersThirdPartyKey.Private.Key},
		LocalUsersThirdPartyPublicKey:     &keyScanner{key: localUsersThirdPartyKey.Public.Key},
		ExternalUsersThirdPartyPrivateKey: &keyScanner{key: externalUsersThirdPartyKey.Private.Key},
		ExternalUsersThirdPartyPublicKey:  &keyScanner{key: externalUsersThirdPartyKey.Public.Key},
		OffersThirdPartyPrivateKey:        &keyScanner{key: offersThirdPartyKey.Private.Key},
		OffersThirdPartyPublicKey:         &keyScanner{key: offersThirdPartyKey.Public.Key},
	}

	initialiseConfigStmt, err := st.Prepare("INSERT INTO bakery_config (*) VALUES ($FullConfig.*)", FullConfig{})
	if err != nil {
		return errors.Errorf("preparing initialise bakery config statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, initialiseConfigStmt, cfg).Run()
		if internaldatabase.IsErrConstraintUnique(err) {
			return macroonerrors.BakeryConfigAlreadyInitialised
		}
		return err
	})
	return errors.Capture(err)
}

// GetLocalUsersKey returns the key pair used with the local users bakery.
func (st *BakeryConfigState) GetLocalUsersKey(ctx context.Context) (*bakery.KeyPair, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &LocalUsersKeyPair.* FROM bakery_config", LocalUsersKeyPair{})
	if err != nil {
		return nil, errors.Errorf("preparing local users key statement: %w", err)
	}

	var keyPair LocalUsersKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		if errors.Is(err, sql.ErrNoRows) {
			return macroonerrors.NotInitialised
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: keyPair.LocalUsersPublicKey.key},
		Private: bakery.PrivateKey{Key: keyPair.LocalUsersPrivateKey.key},
	}, nil
}

// GetLocalUsersThirdPartyKey returns the third party key pair used with the local users bakery.
func (st *BakeryConfigState) GetLocalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &LocalUsersThirdPartyKeyPair.* FROM bakery_config", LocalUsersThirdPartyKeyPair{})
	if err != nil {
		return nil, errors.Errorf("preparing local users third party key statement: %w", err)
	}

	var keyPair LocalUsersThirdPartyKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		if errors.Is(err, sql.ErrNoRows) {
			return macroonerrors.NotInitialised
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	return &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: keyPair.LocalUsersThirdPartyPublicKey.key},
		Private: bakery.PrivateKey{Key: keyPair.LocalUsersThirdPartyPrivateKey.key},
	}, nil
}

// GetExternalUsersThirdPartyKey returns the third party key pair used with the external users bakery.
func (st *BakeryConfigState) GetExternalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &ExternalUsersThirdPartyKeyPair.* FROM bakery_config", ExternalUsersThirdPartyKeyPair{})
	if err != nil {
		return nil, errors.Errorf("preparing external users third party key statement: %w", err)
	}

	var keyPair ExternalUsersThirdPartyKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		if errors.Is(err, sql.ErrNoRows) {
			return macroonerrors.NotInitialised
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	return &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: keyPair.ExternalUsersThirdPartyPublicKey.key},
		Private: bakery.PrivateKey{Key: keyPair.ExternalUsersThirdPartyPrivateKey.key},
	}, nil
}

// GetOffersThirdPartyKey returns the key pair used with the cross model offers bakery.
func (st *BakeryConfigState) GetOffersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &OffersThirdPartyKeyPair.* FROM bakery_config", OffersThirdPartyKeyPair{})
	if err != nil {
		return nil, errors.Errorf("preparing offers third party key statement: %w", err)
	}

	var keyPair OffersThirdPartyKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		if errors.Is(err, sql.ErrNoRows) {
			return macroonerrors.NotInitialised
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: keyPair.OffersThirdPartyPublicKey.key},
		Private: bakery.PrivateKey{Key: keyPair.OffersThirdPartyPrivateKey.key},
	}, nil
}
