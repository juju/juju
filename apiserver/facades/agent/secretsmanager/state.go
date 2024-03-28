// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"
)

type CrossModelState interface {
	GetToken(entity names.Tag) (string, error)
	GetRemoteEntity(token string) (names.Tag, error)
	GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error)
}
