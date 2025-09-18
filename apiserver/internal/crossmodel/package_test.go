// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodel -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodel -destination service_mock_test.go github.com/juju/juju/apiserver/internal/crossmodel AccessService,OfferBakery

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}

func newBakeryMacaroon(c *tc.C, id string) *bakery.Macaroon {
	mac, err := bakery.NewLegacyMacaroon(newMacaroon(c, id))
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
