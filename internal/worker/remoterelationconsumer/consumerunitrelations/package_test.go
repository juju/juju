// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package consumerunitrelations

import (
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package consumerunitrelations -destination service_mock_test.go -source worker.go

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
