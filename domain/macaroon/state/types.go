// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql/driver"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"

	"github.com/juju/juju/internal/errors"
)

// FullConfig holds the full macaroon bakery config data
type FullConfig struct {
	LocalUsersPrivateKey              *keyScanner `db:"local_users_private_key"`
	LocalUsersPublicKey               *keyScanner `db:"local_users_public_key"`
	LocalUsersThirdPartyPrivateKey    *keyScanner `db:"local_users_third_party_private_key"`
	LocalUsersThirdPartyPublicKey     *keyScanner `db:"local_users_third_party_public_key"`
	ExternalUsersThirdPartyPrivateKey *keyScanner `db:"external_users_third_party_private_key"`
	ExternalUsersThirdPartyPublicKey  *keyScanner `db:"external_users_third_party_public_key"`
	OffersThirdPartyPrivateKey        *keyScanner `db:"offers_third_party_private_key"`
	OffersThirdPartyPublicKey         *keyScanner `db:"offers_third_party_public_key"`
}

type LocalUsersKeyPair struct {
	LocalUsersPrivateKey *keyScanner `db:"local_users_private_key"`
	LocalUsersPublicKey  *keyScanner `db:"local_users_public_key"`
}

type LocalUsersThirdPartyKeyPair struct {
	LocalUsersThirdPartyPrivateKey *keyScanner `db:"local_users_third_party_private_key"`
	LocalUsersThirdPartyPublicKey  *keyScanner `db:"local_users_third_party_public_key"`
}

type ExternalUsersThirdPartyKeyPair struct {
	ExternalUsersThirdPartyPrivateKey *keyScanner `db:"external_users_third_party_private_key"`
	ExternalUsersThirdPartyPublicKey  *keyScanner `db:"external_users_third_party_public_key"`
}

type OffersThirdPartyKeyPair struct {
	OffersThirdPartyPrivateKey *keyScanner `db:"offers_third_party_private_key"`
	OffersThirdPartyPublicKey  *keyScanner `db:"offers_third_party_public_key"`
}

// keyScanner wraps a bakery.Key, and implements sql.Scanner
// and driver.Valuer, to allow bakery.Key types to be templated
// in and out of sql via sqlair
type keyScanner struct {
	key bakery.Key
}

// Scan implements the sql.Scanner interface
func (kv *keyScanner) Scan(v any) error {
	switch b := v.(type) {
	case []byte:
		if len(b) != bakery.KeyLen {
			return errors.Errorf("%v is not a valid key, expected length %d", v, bakery.KeyLen)
		}
		copy(kv.key[:], b[:bakery.KeyLen])
	default:
		return errors.Errorf("%v is not a valid byte slice", v)
	}
	return nil
}

// Value implements the driver.Valuer interface
func (kv *keyScanner) Value() (driver.Value, error) {
	return kv.key[:], nil
}

// rootKey holds the state representation of dbrootkeystore.RootKey
// complete with `db` tags for sqlair
type rootKey struct {
	ID      []byte    `db:"id"`
	Created time.Time `db:"created_at"`
	Expires time.Time `db:"expires_at"`
	RootKey []byte    `db:"root_key"`
}

type rootKeyID struct {
	ID []byte `db:"id"`
}
