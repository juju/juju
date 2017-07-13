// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/juju/juju/state"
)

// NewBakery returns a bakery used for minting macaroons used
// in cross model relations.
func NewBakery(st *state.State) (*bakery.Service, error) {
	store, err := st.NewBakeryStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(wallyworld) - shorten the exipry time when macaroon
	// refresh is supported.
	store = store.ExpireAfter(5 * 24 * 365 * time.Hour)
	return bakery.NewService(bakery.NewServiceParams{
		Location: "juju model " + st.ModelUUID(),
		Store:    store,
	})
}
