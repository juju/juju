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

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/crossmodel/mocks"
	coretesting "github.com/juju/juju/testing"
)

type bakerySuite struct {
	testing.IsolationSuite

	mockRoundTripper           *mocks.MockRoundTripper
	mockExpirableStorageBakery *mocks.MockExpirableStorageBakery
}

var _ = gc.Suite(&bakerySuite{})

func (s *bakerySuite) getLocalOfferBakery(c *gc.C) (*crossmodel.OfferBakery, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	mockRoundTripper := mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&crossmodel.DefaultTransport, mockRoundTripper)
	mockBakeryConfig := mocks.NewMockBakeryConfig(ctrl)
	mockExpirableStorage := mocks.NewMockExpirableStorage(ctrl)
	mockFirstPartyCaveatChecker := mocks.NewMockFirstPartyCaveatChecker(ctrl)
	s.mockExpirableStorageBakery = mocks.NewMockExpirableStorageBakery(ctrl)

	key, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	mockBakeryConfig.EXPECT().GetOffersThirdPartyKey().Return(key, nil)
	mockFirstPartyCaveatChecker.EXPECT().Namespace().Return(nil)

	b, err := crossmodel.NewLocalOfferBakery("", mockBakeryConfig, mockExpirableStorage, mockFirstPartyCaveatChecker)
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.NotNil)
	url, err := b.RefreshDischargeURL("https://example.com/offeraccess")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "https://example.com/offeraccess")
	return b, ctrl
}

func (s *bakerySuite) setMockRoundTripperRoundTrip(c *gc.C, expectedUrl string) {
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Content-Type", "application/json")
			if req.URL.String() != "https://example.com/macaroons/discharge/info" {
				return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
			}
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

func (s *bakerySuite) getJaaSOfferBakery(c *gc.C) (*crossmodel.JaaSOfferBakery, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&crossmodel.DefaultTransport, s.mockRoundTripper)
	mockBakeryConfig := mocks.NewMockBakeryConfig(ctrl)
	mockExpirableStorage := mocks.NewMockExpirableStorage(ctrl)
	mockFirstPartyCaveatChecker := mocks.NewMockFirstPartyCaveatChecker(ctrl)

	key, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	mockBakeryConfig.EXPECT().GetExternalUsersThirdPartyKey().Return(key, nil).AnyTimes()
	mockFirstPartyCaveatChecker.EXPECT().Namespace().Return(nil).AnyTimes()
	mockExpirableStorage.EXPECT().ExpireAfter(gomock.Any()).Return(mockExpirableStorage).AnyTimes()
	mockExpirableStorage.EXPECT().RootKey(gomock.Any()).Return(
		[]byte("root-key"), []byte("storage-id"), nil).AnyTimes()

	b, err := crossmodel.NewJaaSOfferBakery(
		"https://example.com/.well-known/jwks.json", "",
		mockBakeryConfig, mockExpirableStorage, mockFirstPartyCaveatChecker,
	)

	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.NotNil)
	return b, ctrl
}

func (s *bakerySuite) TestRefreshDischargeURL(c *gc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	result, err := offerBakery.RefreshDischargeURL("https://example-1.com/offeraccess")
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, "https://example-1.com/offeraccess")
}

func (s *bakerySuite) TestRefreshDischargeURLJaaS(c *gc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	defer ctrl.Finish()

	// Test with no prefixed path segments
	result, err := offerBakery.RefreshDischargeURL("https://example-1.com/.well-known/jwks.json")
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, "https://example-1.com/macaroons")

	// Test with prefixed path segments and assert they're maintained (i.e., ingress rule defines /my-prefix/)
	result, err = offerBakery.RefreshDischargeURL("https://example-2.com/my-prefix/.well-known/jwks.json")
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, "https://example-2.com/my-prefix/macaroons")
}

func (s *bakerySuite) TestGetConsumeOfferCaveats(c *gc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	caveats := offerBakery.GetConsumeOfferCaveats(
		"offer-uuid", "model-uuid", "mary",
	)
	c.Assert(caveats, gc.HasLen, 4)
	c.Assert(strings.HasPrefix(caveats[0].Condition, "time-before"), jc.IsTrue)
	c.Assert(caveats[1], jc.DeepEquals, checkers.Caveat{
		Condition: "declared source-model-uuid model-uuid", Namespace: "std",
	})
	c.Assert(caveats[2], jc.DeepEquals, checkers.Caveat{
		Condition: "declared username mary", Namespace: "std",
	})
	c.Assert(caveats[3], jc.DeepEquals, checkers.Caveat{
		Condition: "declared offer-uuid offer-uuid", Namespace: "std",
	})
}

func (s *bakerySuite) TestGetConsumeOfferCaveatsJaaS(c *gc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	defer ctrl.Finish()

	caveats := offerBakery.GetConsumeOfferCaveats(
		"offer-uuid", "model-uuid", "mary",
	)
	c.Assert(caveats, gc.HasLen, 3)
	c.Assert(strings.HasPrefix(caveats[0].Condition, "time-before"), jc.IsTrue)
	c.Assert(caveats[1], jc.DeepEquals, checkers.Caveat{
		Condition: "declared source-model-uuid model-uuid", Namespace: "std",
	})
	c.Assert(caveats[2], jc.DeepEquals, checkers.Caveat{
		Condition: "declared username mary", Namespace: "std",
	})
}

func (s *bakerySuite) TestInferDeclaredFromMacaroon(c *gc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	mac := apitesting.MustNewMacaroon("test")
	declared := offerBakery.InferDeclaredFromMacaroon(
		macaroon.Slice{mac}, map[string]string{"relation-key": "mediawiki:db mysql:server"},
	)
	c.Assert(declared, gc.DeepEquals, map[string]string{})
}

func (s *bakerySuite) TestInferDeclaredFromMacaroonJaaS(c *gc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	defer ctrl.Finish()

	mac := apitesting.MustNewMacaroon("test")
	declared := offerBakery.InferDeclaredFromMacaroon(
		macaroon.Slice{mac}, map[string]string{"relation-key": "mediawiki:db mysql:server"},
	)
	c.Assert(declared, gc.DeepEquals, map[string]string{"relation-key": "mediawiki:db mysql:server"})
}

func (s *bakerySuite) TestCreateDischargeMacaroon(c *gc.C) {
	offerBakery, ctrl := s.getLocalOfferBakery(c)
	defer ctrl.Finish()

	offerBakery.SetBakery(s.mockExpirableStorageBakery)

	s.mockExpirableStorageBakery.EXPECT().ExpireStorageAfter(gomock.Any()).Return(s.mockExpirableStorageBakery, nil)
	s.mockExpirableStorageBakery.EXPECT().NewMacaroon(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
			sort.Slice(caveats, func(i, j int) bool {
				return caveats[i].Condition < caveats[j].Condition
			})
			c.Assert(caveats, gc.HasLen, 2)
			cavCondition := fmt.Sprintf(`
need-declared offer-uuid,relation-key,source-model-uuid,username,username has-offer-permission source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], coretesting.ModelTag.Id())
			c.Assert(caveats[0], jc.DeepEquals, checkers.Caveat{
				Condition: cavCondition,
				Location:  "https://example.com/offeraccess",
			})
			c.Assert(strings.HasPrefix(caveats[1].Condition, "time-before"), jc.IsTrue)
			c.Assert(ops, gc.HasLen, 1)
			c.Assert(ops[0], jc.DeepEquals, bakery.Op{Action: "consume", Entity: "mysql-uuid"})
			return bakery.NewLegacyMacaroon(apitesting.MustNewMacaroon("test"))
		},
	)
	_, err := offerBakery.CreateDischargeMacaroon(
		context.Background(), "https://example.com/offeraccess", "mary",
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
	c.Assert(err, gc.IsNil)
}

// TestCreateDischargeMacaroonJaaS tests that a macaroon with
// a 3rd part caveat addressed to JAAS can be created, ensuring
// that JAAS' public key is fetched and cached.
func (s *bakerySuite) TestCreateDischargeMacaroonJaaS(c *gc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	s.mockExpirableStorageBakery = mocks.NewMockExpirableStorageBakery(ctrl)
	defer ctrl.Finish()

	s.setMockRoundTripperRoundTrip(c, `https://example.com/macaroons/discharge/info`)

	_, err := s.createTestJaaSMacaroon(c, offerBakery)
	c.Assert(err, gc.IsNil)

	// The second macaroon should use a cached public key
	_, err = s.createTestJaaSMacaroon(c, offerBakery)
	c.Assert(err, gc.IsNil)
}

// TestCreateDischargeMacaroonJaaSUnreachable tests that macaroons
// cannot be created if the controller has created while JAAS is
// unreachable and therefore unable to fetch the public key.
func (s *bakerySuite) TestCreateDischargeMacaroonJaaSUnreachable(c *gc.C) {
	offerBakery, ctrl := s.getJaaSOfferBakery(c)
	s.mockExpirableStorageBakery = mocks.NewMockExpirableStorageBakery(ctrl)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("failed to fetch jaas public key")
		},
	)

	_, err := s.createTestJaaSMacaroon(c, offerBakery)
	c.Assert(err, gc.ErrorMatches, ".*failed to fetch jaas public key.*")
}

func (s *bakerySuite) createTestJaaSMacaroon(_ *gc.C, offerBakery *crossmodel.JaaSOfferBakery) (*bakery.Macaroon, error) {
	return offerBakery.CreateDischargeMacaroon(
		context.Background(), "https://example.com/macaroons", "mary",
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
}
