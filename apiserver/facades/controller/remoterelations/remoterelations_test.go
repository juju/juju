// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/controller/remoterelations"
	"github.com/juju/juju/apiserver/facades/controller/remoterelations/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

func TestRemoteRelationsSuite(t *testing.T) {
	tc.Run(t, &remoteRelationsSuite{})
}

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	authorizer    *apiservertesting.FakeAuthorizer
	ecService     *mocks.MockExternalControllerService
	secretService *mocks.MockSecretService
	cc            *mocks.MockControllerConfigAPI
	api           *remoterelations.API
}

func (s *remoteRelationsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *remoteRelationsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.cc = mocks.NewMockControllerConfigAPI(ctrl)
	s.ecService = mocks.NewMockExternalControllerService(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	api, err := remoterelations.NewRemoteRelationsAPI(
		s.ecService,
		s.secretService,
		s.cc,
		s.authorizer,
	)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *remoteRelationsSuite) TestUpdateControllersForModels(c *tc.C) {
	defer s.setup(c).Finish()

	mod1 := uuid.MustNewUUID().String()
	c1Tag := names.NewControllerTag(uuid.MustNewUUID().String())
	mod2 := uuid.MustNewUUID().String()
	c2Tag := names.NewControllerTag(uuid.MustNewUUID().String())

	c1 := crossmodel.ControllerInfo{
		ControllerUUID: c1Tag.Id(),
		Alias:          "alias1",
		Addrs:          []string{"1.1.1.1:1"},
		CACert:         "cert1",
		ModelUUIDs:     []string{mod1},
	}
	c2 := crossmodel.ControllerInfo{
		ControllerUUID: c2Tag.Id(),
		Alias:          "alias2",
		Addrs:          []string{"2.2.2.2:2"},
		CACert:         "cert2",
		ModelUUIDs:     []string{mod2},
	}

	s.ecService.EXPECT().UpdateExternalController(
		gomock.Any(),
		c1,
	).Return(errors.New("whack"))
	s.ecService.EXPECT().UpdateExternalController(
		gomock.Any(),
		c2,
	).Return(nil)

	res, err := s.api.UpdateControllersForModels(
		c.Context(),
		params.UpdateControllersForModelsParams{
			Changes: []params.UpdateControllerForModel{
				{
					ModelTag: names.NewModelTag(mod1).String(),
					Info: params.ExternalControllerInfo{
						ControllerTag: c1Tag.String(),
						Alias:         "alias1",
						Addrs:         []string{"1.1.1.1:1"},
						CACert:        "cert1",
					},
				},
				{
					ModelTag: names.NewModelTag(mod2).String(),
					Info: params.ExternalControllerInfo{
						ControllerTag: c2Tag.String(),
						Alias:         "alias2",
						Addrs:         []string{"2.2.2.2:2"},
						CACert:        "cert2",
					},
				},
			},
		})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 2)
	c.Assert(res.Results[0].Error.Message, tc.Equals, "whack")
	c.Assert(res.Results[1].Error, tc.IsNil)
}

func (s *remoteRelationsSuite) TestConsumeRemoteSecretChanges(c *tc.C) {
	defer s.setup(c).Finish()

	uri := secrets.NewURI()
	change := params.SecretRevisionChange{
		URI:            uri.String(),
		LatestRevision: 666,
	}
	changes := params.LatestSecretRevisionChanges{
		Changes: []params.SecretRevisionChange{change},
	}

	s.secretService.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666).Return(nil)

	result, err := s.api.ConsumeRemoteSecretChanges(c.Context(), changes)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.IsNil)
}
