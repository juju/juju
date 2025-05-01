// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// credentialTag is the credential tag we're using in the tests.
// needs to fit fmt.Sprintf("%s/%s/%s", cloudName, userName, credentialName)
var credentialTag = names.NewCloudCredentialTag("cloud/user/credential")

type CredentialValidatorSuite struct {
	coretesting.BaseSuite

	authorizer                   apiservertesting.FakeAuthorizer
	cloudService                 *mocks.MockCloudService
	credentialService            *mocks.MockCredentialService
	modelService                 *mocks.MockModelService
	modelInfoService             *mocks.MockModelInfoService
	modelCredentialWatcher       *mocks.MockNotifyWatcher
	modelCredentialWatcherGetter func(ctx context.Context) (watcher.NotifyWatcher, error)
	watcherRegistry              *facademocks.MockWatcherRegistry

	api *credentialvalidator.CredentialValidatorAPI
}

var _ = gc.Suite(&CredentialValidatorSuite{})

func (s *CredentialValidatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *CredentialValidatorSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.cloudService = mocks.NewMockCloudService(ctrl)
	s.credentialService = mocks.NewMockCredentialService(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.modelInfoService = mocks.NewMockModelInfoService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.modelCredentialWatcher = mocks.NewMockNotifyWatcher(ctrl)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	api, err := credentialvalidator.NewCredentialValidatorAPIForTest(c, s.cloudService, s.credentialService, s.authorizer, s.modelService, s.modelInfoService, s.modelCredentialWatcherGetter, s.watcherRegistry)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *CredentialValidatorSuite) TestModelCredential(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	modelInfo := model.ModelInfo{
		UUID:            modelUUID,
		CredentialName:  credentialTag.Name(),
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "user"),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	modelCredentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: modelInfo.CredentialOwner,
		Name:  modelInfo.CredentialName,
	}
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), modelCredentialKey).Return(cloud.Credential{
		Invalid: false,
	}, nil)

	result, err := s.api.ModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ModelCredential{
		Model:           names.NewModelTag(modelUUID.String()).String(),
		Exists:          true,
		CloudCredential: credentialTag.String(),
		Valid:           true,
	})
}

func (s *CredentialValidatorSuite) TestModelCredentialNotNeeded(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	modelInfo := model.ModelInfo{
		UUID:            modelUUID,
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "user"),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)

	s.cloudService.EXPECT().Cloud(gomock.Any(), modelInfo.Cloud).Return(&cloud.Cloud{
		Name:      modelInfo.Cloud,
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	}, nil)

	result, err := s.api.ModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ModelCredential{Model: names.NewModelTag(modelUUID.String()).String(), Valid: true})
}

func (s *CredentialValidatorSuite) TestWatchCredential(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	modelInfo := model.ModelInfo{
		UUID:            modelUUID,
		CredentialName:  credentialTag.Name(),
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "user"),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	modelCredentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: modelInfo.CredentialOwner,
		Name:  modelInfo.CredentialName,
	}

	ch := make(chan struct{}, 1)
	ch <- struct{}{}

	s.modelCredentialWatcher.EXPECT().Changes().Return(ch)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	s.credentialService.EXPECT().WatchCredential(gomock.Any(), modelCredentialKey).Return(s.modelCredentialWatcher, nil)

	modelCredentialTag, err := modelCredentialKey.Tag()
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.WatchCredential(context.Background(), params.Entity{Tag: modelCredentialTag.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "1", Error: nil})
}

func (s *CredentialValidatorSuite) TestWatchCredentialNotUsedInThisModel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	modelInfo := model.ModelInfo{
		UUID:            modelUUID,
		CredentialName:  "not-tag-credential",
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "user"),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	_, err := s.api.WatchCredential(context.Background(), params.Entity{"cloudcred-cloud_fred_default"})
	c.Assert(err, gc.ErrorMatches, apiservererrors.ErrPerm.Error())
}

func (s *CredentialValidatorSuite) TestWatchCredentialInvalidTag(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	_, err := s.api.WatchCredential(context.Background(), params.Entity{"my-tag"})
	c.Assert(err, gc.ErrorMatches, `"my-tag" is not a valid tag`)
}

func (s *CredentialValidatorSuite) TestInvalidateModelCredential(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	modelInfo := model.ModelInfo{
		UUID:            modelUUID,
		CredentialName:  credentialTag.Name(),
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "user"),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	modelCredentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: modelInfo.CredentialOwner,
		Name:  modelInfo.CredentialName,
	}
	reason := "not again"
	s.credentialService.EXPECT().InvalidateCredential(gomock.Any(), modelCredentialKey, reason).Return(nil)

	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{Reason: reason})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
}

func (s *CredentialValidatorSuite) TestInvalidateModelCredentialError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	modelInfo := model.ModelInfo{
		UUID:            modelUUID,
		CredentialName:  credentialTag.Name(),
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "user"),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	modelCredentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: modelInfo.CredentialOwner,
		Name:  modelInfo.CredentialName,
	}
	reason := "not again"
	s.credentialService.EXPECT().InvalidateCredential(gomock.Any(), modelCredentialKey, reason).Return(coreerrors.NotValid)

	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{Reason: reason})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: apiservererrors.ServerError(coreerrors.NotValid)})
}

func (s *CredentialValidatorSuite) TestWatchModelCredential(c *gc.C) {
	s.modelCredentialWatcherGetter = func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return s.modelCredentialWatcher, nil
	}
	defer s.setUpMocks(c).Finish()
	ch := make(chan struct{}, 1)
	ch <- struct{}{}

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	s.modelCredentialWatcher.EXPECT().Changes().Return(ch)

	result, err := s.api.WatchModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
}

func (s *CredentialValidatorSuite) TestWatchModelCredentialError(c *gc.C) {
	s.modelCredentialWatcherGetter = func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return nil, coreerrors.NotValid
	}
	defer s.setUpMocks(c).Finish()
	_, err := s.api.WatchModelCredential(context.Background())
	c.Assert(err, gc.DeepEquals, apiservererrors.ServerError(coreerrors.NotValid))
}
