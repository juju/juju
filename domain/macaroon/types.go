// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package macaroon

import (
	"time"
)

// RootKey holds the internal representation of dbrootkeystore.rootKey
type RootKey struct {
	ID      []byte
	Created time.Time
	Expires time.Time
	RootKey []byte
}
