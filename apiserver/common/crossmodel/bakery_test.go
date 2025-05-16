// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/crossmodel/mocks"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type bakerySuite struct {
	testhelpers.IsolationSuite

	mockRoundTripper           *mocks.MockRoundTripper
	mockExpirableStorageBakery *mocks.MockExpirableStorageBakery
}

func TestBakerySuite(t *stdtesting.T) { tc.Run(t, &bakerySuite{}) }
func (s *bakerySuite) getLocalOfferBakery(c *tc.C) (*crossmodel.OfferBakery, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	mockRoundTripper := mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&crossmodel.DefaultTransport, mockRoundTripper)
	mockExpirableStorage := mocks.NewMockExpirableStorage(ctrl)
	mockFirstPartyCaveatChecker := mocks.NewMockFirstPartyCaveatChecker(ctrl)
	s.mockExpirableStorageBakery = mocks.NewMockExpirableStorageBakery(ctrl)

	key, err := bakery.GenerateKey()
	c.Assert(err, tc.IsNil)
	mockFirstPartyCaveatChecker.EXPECT().Namespace().Return(nil)

	b, err := crossmodel.NewLocalOfferBakery("", key, mockExpirableStorage, mockFirstPartyCaveatChecker, clock.WallClock)
	c.Assert(err, tc.IsNil)
	c.Assert(b, tc.NotNil)
	url, err := b.RefreshDischargeURL(c.Context(), "https://example.com/offeraccess")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url, tc.Equals, "https://example.com/offeraccess")
	return b, ctrl
}

func (s *bakerySuite) setMockRoundTripperRoundTrip(c *tc.C, expectedUrl string) {
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Content-Type", "application/json")
			c.Assert(req.URL.String(), tc.Equals, expectedUrl)
			resp := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body: io.NopCloser(
					strings.NewReader(
						`{"PublicKey": "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=", "Version": 3}`,
					),
				),
			}
			resp.Header = req.Header
			return resp, nil
		},
	).Times(1)
}

func (s *bakerySuite) getJaaSOfferBakery(c *tc.C) (*crossmodel.JaaSOfferBakery, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&crossmodel.DefaultTransport, s.mockRoundTripper)
	mockBakeryConfig := mocks.NewMockBakeryConfigService(ctrl)
	mockExpirableStorage := mocks.NewMockExpirableStorage(ctrl)
	mockFirstPartyCaveatChecker := mocks.NewMockFirstPartyCaveatChecker(ctrl)

	key, err := bakery.GenerateKey()
	c.Assert(err, tc.IsNil)
	mockBakeryConfig.EXPECT().GetExternalUsersThirdPartyKey(gomock.Any()).Return(key, nil).AnyTimes()
	mockFirstPartyCaveatChecker.EXPECT().Namespace().Return(nil).AnyTimes()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Content-Type", "application/json")
			c.Assert(req.URL.String(), tc.Equals, `https://example.com/macaroons/discharge/info`)
			resp := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body: io.NopCloser(
					strings.NewReader(
						`{"PublicKey": "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=", "Version": 3}`,
					),
				),
			}
			resp.Header = req.Header
			return resp, nil
		},
	)

	b, err := crossmodel.NewJaaSOfferBakery(
		c.Context(),
		"https://example.com/.well-known/jwks.json", "",
		clock.WallClock,
		mockBakeryConfig, mockExpirableStorage, mockFirstPartyCaveatChecker,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(b, tc.NotNil)
	return b, ctrl
}

func (s *bakerySuite) TestRefreshDischargeURL(c *tc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	result, err := offerBakery.RefreshDischargeURL(c.Context(), "https://example-1.com/offeraccess")
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, "https://example-1.com/offeraccess")
}

func (s *bakerySuite) TestRefreshDischargeURLJaaS(c *tc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	defer ctrl.Finish()

	// Test with no prefixed path segments
	s.setMockRoundTripperRoundTrip(c, `https://example-1.com/macaroons/discharge/info`)
	result, err := offerBakery.RefreshDischargeURL(c.Context(), "https://example-1.com/.well-known/jwks.json")
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, "https://example-1.com/macaroons")

	// Test with prefixed path segments and assert they're maintained (i.e., ingress rule defines /my-prefix/)
	s.setMockRoundTripperRoundTrip(c, `https://example-2.com/my-prefix/macaroons/discharge/info`)
	result, err = offerBakery.RefreshDischargeURL(c.Context(), "https://example-2.com/my-prefix/.well-known/jwks.json")
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, "https://example-2.com/my-prefix/macaroons")
}

func (s *bakerySuite) TestGetConsumeOfferCaveats(c *tc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	caveats := offerBakery.GetConsumeOfferCaveats(
		"offer-uuid", "model-uuid", "mary",
	)
	c.Assert(caveats, tc.HasLen, 4)
	c.Assert(strings.HasPrefix(caveats[0].Condition, "time-before"), tc.IsTrue)
	c.Assert(caveats[1], tc.DeepEquals, checkers.Caveat{
		Condition: "declared source-model-uuid model-uuid", Namespace: "std",
	})
	c.Assert(caveats[2], tc.DeepEquals, checkers.Caveat{
		Condition: "declared username mary", Namespace: "std",
	})
	c.Assert(caveats[3], tc.DeepEquals, checkers.Caveat{
		Condition: "declared offer-uuid offer-uuid", Namespace: "std",
	})
}

func (s *bakerySuite) TestGetConsumeOfferCaveatsJaaS(c *tc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	defer ctrl.Finish()

	caveats := offerBakery.GetConsumeOfferCaveats(
		"offer-uuid", "model-uuid", "mary",
	)
	c.Assert(caveats, tc.HasLen, 3)
	c.Assert(strings.HasPrefix(caveats[0].Condition, "time-before"), tc.IsTrue)
	c.Assert(caveats[1], tc.DeepEquals, checkers.Caveat{
		Condition: "declared source-model-uuid model-uuid", Namespace: "std",
	})
	c.Assert(caveats[2], tc.DeepEquals, checkers.Caveat{
		Condition: "declared username mary", Namespace: "std",
	})
}

func (s *bakerySuite) TestInferDeclaredFromMacaroon(c *tc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	mac := jujutesting.MustNewMacaroon("test")
	declared := offerBakery.InferDeclaredFromMacaroon(
		macaroon.Slice{mac}, map[string]string{"relation-key": "mediawiki:db mysql:server"},
	)
	c.Assert(declared, tc.DeepEquals, map[string]string{})
}

func (s *bakerySuite) TestInferDeclaredFromMacaroonJaaS(c *tc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	defer ctrl.Finish()

	mac := jujutesting.MustNewMacaroon("test")
	declared := offerBakery.InferDeclaredFromMacaroon(
		macaroon.Slice{mac}, map[string]string{"relation-key": "mediawiki:db mysql:server"},
	)
	c.Assert(declared, tc.DeepEquals, map[string]string{"relation-key": "mediawiki:db mysql:server"})
}

func (s *bakerySuite) TestCreateDischargeMacaroon(c *tc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	offerBakery.SetBakery(s.mockExpirableStorageBakery)

	s.mockExpirableStorageBakery.EXPECT().ExpireStorageAfter(gomock.Any()).Return(s.mockExpirableStorageBakery, nil)
	s.mockExpirableStorageBakery.EXPECT().NewMacaroon(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
			sort.Slice(caveats, func(i, j int) bool {
				return caveats[i].Condition < caveats[j].Condition
			})
			c.Assert(caveats, tc.HasLen, 2)
			cavCondition := fmt.Sprintf(`
need-declared offer-uuid,relation-key,source-model-uuid,username,username has-offer-permission source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], coretesting.ModelTag.Id())
			c.Assert(caveats[0], tc.DeepEquals, checkers.Caveat{
				Condition: cavCondition,
				Location:  "https://example.com/offeraccess",
			})
			c.Assert(strings.HasPrefix(caveats[1].Condition, "time-before"), tc.IsTrue)
			c.Assert(ops, tc.HasLen, 1)
			c.Assert(ops[0], tc.DeepEquals, bakery.Op{Action: "consume", Entity: "mysql-uuid"})
			return bakery.NewLegacyMacaroon(jujutesting.MustNewMacaroon("test"))
		},
	)
	_, err := offerBakery.CreateDischargeMacaroon(
		c.Context(), "https://example.com/offeraccess", "mary",
		map[string]string{
			"relation-key":      "mediawiki:db mysql:server",
			"username":          "mary",
			"offer-uuid":        "mysql-uuid",
			"source-model-uuid": coretesting.ModelTag.Id(),
		},
		map[string]string{
			"relation-key":      "mediawiki:db mysql:server",
			"username":          "mary",
			"source-model-uuid": coretesting.ModelTag.Id(),
		},
		bakery.Op{Action: "consume", Entity: "mysql-uuid"},
		bakery.LatestVersion,
	)
	c.Assert(err, tc.IsNil)
}

func (s *bakerySuite) TestCreateDischargeMacaroonJaaS(c *tc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	s.mockExpirableStorageBakery = mocks.NewMockExpirableStorageBakery(ctrl)
	defer ctrl.Finish()

	offerBakery.SetBakery(s.mockExpirableStorageBakery)

	s.mockExpirableStorageBakery.EXPECT().ExpireStorageAfter(gomock.Any()).Return(s.mockExpirableStorageBakery, nil)
	s.mockExpirableStorageBakery.EXPECT().NewMacaroon(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
			sort.Slice(caveats, func(i, j int) bool {
				return caveats[i].Condition < caveats[j].Condition
			})
			c.Assert(caveats, tc.HasLen, 5)
			c.Assert(caveats[0], tc.DeepEquals, checkers.Caveat{
				Condition: "declared relation-key mediawiki:db mysql:server", Namespace: "std",
			})
			c.Assert(caveats[1], tc.DeepEquals, checkers.Caveat{
				Condition: "declared source-model-uuid " + coretesting.ModelTag.Id(), Namespace: "std",
			})
			c.Assert(caveats[2], tc.DeepEquals, checkers.Caveat{
				Condition: "declared username mary", Namespace: "std",
			})
			c.Assert(caveats[3], tc.DeepEquals, checkers.Caveat{
				Location: "https://example.com/macaroons", Condition: "is-consumer user-mary mysql-uuid",
			})
			c.Assert(strings.HasPrefix(caveats[4].Condition, "time-before"), tc.IsTrue)
			return bakery.NewLegacyMacaroon(jujutesting.MustNewMacaroon("test"))
		},
	)
	_, err := offerBakery.CreateDischargeMacaroon(
		c.Context(), "https://example.com/macaroons", "mary",
		map[string]string{
			"relation-key":      "mediawiki:db mysql:server",
			"username":          "mary",
			"offer-uuid":        "mysql-uuid",
			"source-model-uuid": coretesting.ModelTag.Id(),
		},
		map[string]string{
			"relation-key":      "mediawiki:db mysql:server",
			"username":          "mary",
			"source-model-uuid": coretesting.ModelTag.Id(),
		},
		bakery.Op{Action: "consume", Entity: "mysql-uuid"},
		bakery.LatestVersion,
	)
	c.Assert(err, tc.IsNil)
}
