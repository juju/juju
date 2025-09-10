// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
)

type jaasBakerySuite struct {
	baseBakerySuite
}

func TestJAASBakerySuite(t *testing.T) {
	tc.Run(t, &jaasBakerySuite{})
}

func (s *localBakerySuite) TestNewJAASOfferBakery(c *tc.C) {
	defer s.setupMocks(c).Finish()

	checker := checkers.New(internalmacaroon.MacaroonNamespace)
	bakery, err := NewJAASOfferBakery(
		s.keyPair,
		"juju model",
		"http://offer-access",
		s.store,
		checker,
		s.authorizer,
		s.httpClient,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(bakery, tc.Not(tc.IsNil))
}

func (s *jaasBakerySuite) TestParseCaveatNoOfferPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := JAASOfferBakery{}
	_, err := bakery.ParseCaveat("naughty-caveat")
	c.Check(err, tc.Equals, checkers.ErrCaveatNotRecognized)
}

func (s *jaasBakerySuite) TestParseCaveatInvalidYAML(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := JAASOfferBakery{}
	_, err := bakery.ParseCaveat("has-offer-permission invalid-yaml")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *jaasBakerySuite) TestParseCaveat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := JAASOfferBakery{}
	details, err := bakery.ParseCaveat("has-offer-permission " + s.newAccessCaveat(s.modelUUID.String()))
	c.Check(err, tc.ErrorIsNil)
	c.Check(details, tc.DeepEquals, OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	})
}

func (s *jaasBakerySuite) TestGetConsumeOfferCaveats(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	bakery := JAASOfferBakery{
		clock: s.clock,
	}
	caveats := bakery.GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "")

	c.Check(caveats, tc.SameContents, s.caveats(now))
}

func (s *jaasBakerySuite) TestGetConsumeOfferCaveatsWithRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// JAAS ignores the relation key when looking at consume offer caveats.
	// Instead it comes from the inferred declared values.

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	bakery := JAASOfferBakery{
		clock: s.clock,
	}
	caveats := bakery.GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "mediawiki:db mysql:server")

	c.Check(caveats, tc.SameContents, s.caveats(now))
}

func (s *jaasBakerySuite) TestInferDeclaredFromMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// JAAS bakery returns the relation key from the inferred declared values.

	mac := newMacaroon(c, "test")

	bakery := JAASOfferBakery{
		logger: loggertesting.WrapCheckLog(c),
	}
	m := bakery.InferDeclaredFromMacaroon(macaroon.Slice{mac}, map[string]string{"relation-key": "mediawiki:db mysql:server"})
	c.Check(m, tc.DeepEquals, map[string]string{
		relationKey: "mediawiki:db mysql:server",
	})
}

func (s *jaasBakerySuite) caveats(now time.Time) []checkers.Caveat {
	return []checkers.Caveat{
		checkers.DeclaredCaveat(sourceModelKey, s.modelUUID.String()),
		checkers.DeclaredCaveat(usernameKey, "mary"),
		checkers.TimeBeforeCaveat(now.Add(offerPermissionExpiryTime)),
	}
}

func (s *jaasBakerySuite) TestCreateDischargeMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	expectedMac := newBakeryMacaroon(c, "test")
	op := bakery.Op{Action: "consume", Entity: "mysql-uuid"}

	var caveats []checkers.Caveat
	s.oven.EXPECT().NewMacaroon(
		gomock.Any(),
		bakery.LatestVersion,
		gomock.Any(),
		op,
	).DoAndReturn(func(ctx context.Context, v bakery.Version, c []checkers.Caveat, o ...bakery.Op) (*bakery.Macaroon, error) {
		caveats = c
		return expectedMac, nil
	})

	localBakery := JAASOfferBakery{
		oven:  s.oven,
		clock: s.clock,
	}

	mac, err := localBakery.CreateDischargeMacaroon(
		c.Context(),
		"http://offer-access",
		"mary",
		map[string]string{
			sourceModelKey: s.modelUUID.String(),
			relationKey:    "mediawiki:db mysql:server",
			offerUUIDKey:   "mysql-uuid",
		},
		map[string]string{},
		op,
		bakery.LatestVersion,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mac, tc.Equals, expectedMac)

	c.Check(caveats, tc.DeepEquals, []checkers.Caveat{
		{
			Location:  "http://offer-access",
			Condition: "is-consumer user-mary mysql-uuid",
		},
		checkers.TimeBeforeCaveat(now.Add(offerPermissionExpiryTime)),
	})
}

func (s *jaasBakerySuite) newAccessCaveat(modelUUID string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], modelUUID)
}

var _ bakery.ThirdPartyLocator = (*externalPublicKeyLocator)(nil)

type externalPublicKeyLocatorSuite struct {
	httpClient *MockHTTPClient
}

func TestExternalPublicKeyLocatorSuite(t *testing.T) {
	tc.Run(t, &externalPublicKeyLocatorSuite{})
}

func (s *externalPublicKeyLocatorSuite) TestThirdPartyInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		req.Header.Set("Content-Type", "application/json")
		return &http.Response{
			Request:    req,
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(
				strings.NewReader(
					`{"PublicKey": "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=", "Version": 3}`,
				),
			),
		}, nil
	})

	locator := newExternalPublicKeyLocator("http://offer-access", s.httpClient, loggertesting.WrapCheckLog(c))

	info, err := locator.ThirdPartyInfo(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.PublicKey.String(), tc.Equals, "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=")
	c.Check(info.Version, tc.Equals, bakery.Version(3))
}

func (s *externalPublicKeyLocatorSuite) TestThirdPartyInfoCaching(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Notice only one HTTP request is expected.

	s.httpClient.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		req.Header.Set("Content-Type", "application/json")
		return &http.Response{
			Request:    req,
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(
				strings.NewReader(
					`{"PublicKey": "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=", "Version": 3}`,
				),
			),
		}, nil
	})

	locator := newExternalPublicKeyLocator("http://offer-access", s.httpClient, loggertesting.WrapCheckLog(c))

	info, err := locator.ThirdPartyInfo(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.PublicKey.String(), tc.Equals, "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=")
	c.Check(info.Version, tc.Equals, bakery.Version(3))

	info, err = locator.ThirdPartyInfo(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.PublicKey.String(), tc.Equals, "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=")
	c.Check(info.Version, tc.Equals, bakery.Version(3))
}

func (s *externalPublicKeyLocatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.httpClient = NewMockHTTPClient(ctrl)

	c.Cleanup(func() {
		s.httpClient = nil
	})

	return ctrl
}
