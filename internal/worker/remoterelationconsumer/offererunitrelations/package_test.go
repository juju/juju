// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offererunitrelations

import (
	"github.com/juju/tc"
	macaroon "gopkg.in/macaroon.v2"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package offererunitrelations -destination client_mock_test.go -source worker.go

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}

func ptr[T any](v T) *T {
	return &v
}
