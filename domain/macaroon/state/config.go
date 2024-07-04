// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	internaldatabase "github.com/juju/juju/internal/database"
)

// BakeryConfigState describes the persistence layer to
// store bakery config
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
		return errors.Trace(err)
	}

	cfg := FullConfig{
		LocalUsersPrivateKey:              &keyScanner{localUsersKey.Private.Key},
		LocalUsersPublicKey:               &keyScanner{localUsersKey.Public.Key},
		LocalUsersThirdPartyPrivateKey:    &keyScanner{localUsersThirdPartyKey.Private.Key},
		LocalUsersThirdPartyPublicKey:     &keyScanner{localUsersThirdPartyKey.Public.Key},
		ExternalUsersThirdPartyPrivateKey: &keyScanner{externalUsersThirdPartyKey.Private.Key},
		ExternalUsersThirdPartyPublicKey:  &keyScanner{externalUsersThirdPartyKey.Public.Key},
		OffersThirdPartyPrivateKey:        &keyScanner{offersThirdPartyKey.Private.Key},
		OffersThirdPartyPublicKey:         &keyScanner{offersThirdPartyKey.Public.Key},
	}

	initialiseConfigStmt, err := st.Prepare("INSERT INTO bakery_config (*) VALUES ($FullConfig.*)", FullConfig{})
	if err != nil {
		return errors.Annotate(err, "preparing initialise bakery config statement")
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, initialiseConfigStmt, cfg).Run()
		if internaldatabase.IsErrConstraintUnique(err) {
			return BakeryConfigAlreadyInitialised
		}
		return domain.CoerceError(err)
	})
	return errors.Trace(err)
}

// GetLocalUsersKey returns the key pair used with the local users bakery.
func (st *BakeryConfigState) GetLocalUsersKey(ctx context.Context) (*bakery.KeyPair, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &LocalUsersKeyPair.* FROM bakery_config", LocalUsersKeyPair{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing local users key statement")
	}

	var keyPair LocalUsersKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.NotYetAvailablef("bakery config not yet initialised")
	}
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
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
		return nil, errors.Trace(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &LocalUsersThirdPartyKeyPair.* FROM bakery_config", LocalUsersThirdPartyKeyPair{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing local users third party key statement")
	}

	var keyPair LocalUsersThirdPartyKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.NotYetAvailablef("bakery config not yet initialised")
	}
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
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
		return nil, errors.Trace(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &ExternalUsersThirdPartyKeyPair.* FROM bakery_config", ExternalUsersThirdPartyKeyPair{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing external users third party key statement")
	}

	var keyPair ExternalUsersThirdPartyKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.NotYetAvailablef("bakery config not yet initialised")
	}
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
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
		return nil, errors.Trace(err)
	}

	loadKeyStmt, err := st.Prepare("SELECT &OffersThirdPartyKeyPair.* FROM bakery_config", OffersThirdPartyKeyPair{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing offers third party key statement")
	}

	var keyPair OffersThirdPartyKeyPair
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, loadKeyStmt).Get(&keyPair)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.NotYetAvailablef("bakery config not yet initialised")
	}
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	return &bakery.KeyPair{
		Public:  bakery.PublicKey{Key: keyPair.OffersThirdPartyPublicKey.key},
		Private: bakery.PrivateKey{Key: keyPair.OffersThirdPartyPrivateKey.key},
	}, nil
}
