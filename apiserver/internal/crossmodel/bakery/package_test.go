// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bakery -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package bakery -destination service_mock_test.go github.com/juju/juju/apiserver/internal/crossmodel/bakery BakeryStore,Oven,HTTPClient
//go:generate go run go.uber.org/mock/mockgen -typed -package bakery -destination bakery_mock_test.go github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery OpsAuthorizer

type baseBakerySuite struct {
	modelUUID model.UUID

	clock      *MockClock
	store      *MockBakeryStore
	oven       *MockOven
	authorizer *MockOpsAuthorizer
	httpClient *MockHTTPClient

	keyPair *bakery.KeyPair
}

func (s *baseBakerySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.store = NewMockBakeryStore(ctrl)
	s.oven = NewMockOven(ctrl)
	s.authorizer = NewMockOpsAuthorizer(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)

	s.modelUUID = modeltesting.GenModelUUID(c)

	s.keyPair = bakery.MustGenerateKey()

	c.Cleanup(func() {
		s.clock = nil
		s.store = nil
		s.oven = nil
		s.authorizer = nil
		s.httpClient = nil

		s.modelUUID = ""

		s.keyPair = nil
	})

	return ctrl
}

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
