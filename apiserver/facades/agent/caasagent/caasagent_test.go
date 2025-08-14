// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

func TestCaasagentSuite(t *testing.T) {
	tc.Run(t, &caasagentSuite{})
}

type caasagentSuite struct {
	coretesting.BaseSuite

	registry facade.WatcherRegistry

	modelUUID coremodel.UUID

	modelService                 *MockModelService
	modelConfigService           *MockModelConfigService
	controllerConfigService      *MockControllerConfigService
	apiHostPortsForAgentsGetter  *MockAPIHostPortsForAgentsGetter
	externalControllerService    *MockExternalControllerService
	modelProviderServicebService *MockModelProviderService

	facade *caasagent.FacadeV2
	result cloudspec.CloudSpec
}

func (s *caasagentSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.registry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)

	s.modelUUID = coremodel.GenUUID(c)

	credential := cloud.NewCredential("auth-type", map[string]string{"k": "v"})
	s.result = cloudspec.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential:       &credential,
		CACertificates:   []string{coretesting.CACert},
		SkipTLSVerify:    true,
	}
}

func (s *caasagentSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelProviderServicebService = NewMockModelProviderService(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.apiHostPortsForAgentsGetter = NewMockAPIHostPortsForAgentsGetter(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.externalControllerService = NewMockExternalControllerService(ctrl)

	modelConfigAPI := model.NewModelConfigWatcher(
		s.modelConfigService, s.registry,
	)
	s.facade = caasagent.NewFacadeV2(
		s.modelUUID, s.registry, modelConfigAPI,
		nil,
		s.modelProviderServicebService,
		func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return s.modelService.WatchModelCloudCredential(ctx, s.modelUUID)
		})

	return ctrl
}

func (s *caasagentSuite) TestPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authorizer := &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("someapp"),
	}

	_, err := caasagent.NewFacadeV2AuthCheck(facadetest.ModelContext{
		Auth_:      authorizer,
		ModelUUID_: s.modelUUID,
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *caasagentSuite) TestCloudSpec(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelProviderServicebService.EXPECT().GetCloudSpec(gomock.Any()).Return(s.result, nil)

	otherModelTag := names.NewModelTag(coremodel.GenUUID(c).String())
	machineTag := names.NewMachineTag("42")
	result, err := s.facade.CloudSpec(
		c.Context(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
			{otherModelTag.String()},
			{machineTag.String()},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.CloudSpecResult{{
		Result: &params.CloudSpec{
			Type:             "type",
			Name:             "name",
			Region:           "region",
			Endpoint:         "endpoint",
			IdentityEndpoint: "identity-endpoint",
			StorageEndpoint:  "storage-endpoint",
			Credential: &params.CloudCredential{
				AuthType:   "auth-type",
				Attributes: map[string]string{"k": "v"},
			},
			CACertificates: []string{coretesting.CACert},
			SkipTLSVerify:  true,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeUnauthorized,
			Message: "permission denied",
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})
}

func (s *caasagentSuite) TestCloudSpecCloudSpecError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelProviderServicebService.EXPECT().GetCloudSpec(gomock.Any()).Return(cloudspec.CloudSpec{}, errors.New("error"))

	result, err := s.facade.CloudSpec(
		c.Context(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.CloudSpecResult{{
		Error: &params.Error{
			Message: `error`,
		},
	}})
}

func (s *caasagentSuite) TestWatchCloudSpecsChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{}, 1)
	// Initial event.
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)
	s.modelService.EXPECT().WatchModelCloudCredential(gomock.Any(), s.modelUUID).Return(w, nil)

	otherModelTag := names.NewModelTag(uuid.MustNewUUID().String())
	machineTag := names.NewMachineTag("42")
	result, err := s.facade.WatchCloudSpecsChanges(
		c.Context(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
			{otherModelTag.String()},
			{machineTag.String()},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.NotifyWatchResult{{
		NotifyWatcherId: "w-1",
	}, {
		Error: &params.Error{
			Code:    params.CodeUnauthorized,
			Message: "permission denied",
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})
}

func (s *caasagentSuite) TestCloudSpecNilCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.result.Credential = nil

	s.modelProviderServicebService.EXPECT().GetCloudSpec(gomock.Any()).Return(s.result, nil)

	result, err := s.facade.CloudSpec(
		c.Context(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.CloudSpecResult{{
		Result: &params.CloudSpec{
			Type:             "type",
			Name:             "name",
			Region:           "region",
			Endpoint:         "endpoint",
			IdentityEndpoint: "identity-endpoint",
			StorageEndpoint:  "storage-endpoint",
			CACertificates:   []string{coretesting.CACert},
			SkipTLSVerify:    true,
		},
	}})
}
