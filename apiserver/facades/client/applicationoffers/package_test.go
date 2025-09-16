// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package applicationoffers -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package applicationoffers -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/applicationoffers AccessService,ModelService,CrossModelRelationService,RemovalService,CrossModelAuthContext,ControllerService

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
