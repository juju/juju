// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/idmclient/v2/ussologin"

	"github.com/juju/juju/juju/osenv"
)

// NewTokenStore returns a FileTokenStore for storing the USSO oauth token
func NewTokenStore() *ussologin.FileTokenStore {
	return ussologin.NewFileTokenStore(osenv.JujuXDGDataHomePath("store-usso-token"))
}
