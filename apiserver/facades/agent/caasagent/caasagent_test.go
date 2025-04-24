// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&caasagentSuite{})

type caasagentSuite struct {
	coretesting.BaseSuite

	registry facade.WatcherRegistry

	modelUUID coremodel.UUID

	modelService                 *MockModelService
	modelConfigService           *MockModelConfigService
	controllerConfigService      *MockControllerConfigService
	externalControllerService    *MockExternalControllerService
	controllerConfigState        *MockControllerConfigState
	modelProviderServicebService *MockModelProviderService

	facade *caasagent.FacadeV2
	result cloudspec.CloudSpec
}

func (s *caasagentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.registry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)

	s.modelUUID = modeltesting.GenModelUUID(c)

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

func (s *caasagentSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelProviderServicebService = NewMockModelProviderService(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.controllerConfigState = NewMockControllerConfigState(ctrl)

	controllerConfigAPI := common.NewControllerConfigAPI(
		s.controllerConfigState,
		s.controllerConfigService,
		s.externalControllerService,
	)
	modelConfigAPI := model.NewModelConfigWatcher(
		s.modelConfigService, s.registry,
	)
	s.facade = caasagent.NewFacadeV2(
		s.modelUUID, s.registry, modelConfigAPI,
		controllerConfigAPI,
		s.modelProviderServicebService,
		func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return s.modelService.WatchModelCloudCredential(ctx, s.modelUUID)
		})

	return ctrl
}

func (s *caasagentSuite) TestPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authorizer := &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("someapp"),
	}

	_, err := caasagent.NewFacadeV2AuthCheck(facadetest.ModelContext{
		Auth_:      authorizer,
		ModelUUID_: s.modelUUID,
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *caasagentSuite) TestCloudSpec(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelProviderServicebService.EXPECT().GetCloudSpec(gomock.Any()).Return(s.result, nil)

	otherModelTag := names.NewModelTag(modeltesting.GenModelUUID(c).String())
	machineTag := names.NewMachineTag("42")
	result, err := s.facade.CloudSpec(
		context.Background(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
			{otherModelTag.String()},
			{machineTag.String()},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.CloudSpecResult{{
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

func (s *caasagentSuite) TestCloudSpecCloudSpecError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelProviderServicebService.EXPECT().GetCloudSpec(gomock.Any()).Return(cloudspec.CloudSpec{}, errors.New("error"))

	result, err := s.facade.CloudSpec(
		context.Background(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.CloudSpecResult{{
		Error: &params.Error{
			Message: `error`,
		},
	}})
}

func (s *caasagentSuite) TestWatchCloudSpecsChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{}, 1)
	// Initial event.
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)
	s.modelService.EXPECT().WatchModelCloudCredential(gomock.Any(), s.modelUUID).Return(w, nil)

	otherModelTag := names.NewModelTag(uuid.MustNewUUID().String())
	machineTag := names.NewMachineTag("42")
	result, err := s.facade.WatchCloudSpecsChanges(
		context.Background(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
			{otherModelTag.String()},
			{machineTag.String()},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.NotifyWatchResult{{
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

func (s *caasagentSuite) TestCloudSpecNilCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.result.Credential = nil

	s.modelProviderServicebService.EXPECT().GetCloudSpec(gomock.Any()).Return(s.result, nil)

	result, err := s.facade.CloudSpec(
		context.Background(),
		params.Entities{Entities: []params.Entity{
			{names.NewModelTag(s.modelUUID.String()).String()},
		}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.CloudSpecResult{{
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
