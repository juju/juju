// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type modelmanagerSuite struct {
}

func TestModelmanagerSuite(t *testing.T) {
	tc.Run(t, &modelmanagerSuite{})
}

func (s *modelmanagerSuite) TestCreateModelBadUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.CreateModel(c.Context(), "mymodel", "not a qualifier", "", "", names.CloudCredentialTag{}, nil)
	c.Assert(err, tc.ErrorMatches, `invalid qualifier "not a qualifier"`)
}

func (s *modelmanagerSuite) TestCreateModelBadCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.CreateModel(c.Context(), "mymodel", "bob", "123!", "", names.CloudCredentialTag{}, nil)
	c.Assert(err, tc.ErrorMatches, `invalid cloud name "123!"`)
}

func (s *modelmanagerSuite) TestCreateModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:        "new-model",
		Qualifier:   "prod",
		Config:      map[string]interface{}{"abc": 123},
		CloudTag:    "cloud-nimbus",
		CloudRegion: "catbus",
	}

	result := new(params.ModelInfo)
	ress := params.ModelInfo{}
	ress.Name = "dowhatimean"
	ress.Type = "iaas"
	ress.UUID = "youyoueyedee"
	ress.ControllerUUID = "youyoueyedeetoo"
	ress.ProviderType = "C-123"
	ress.CloudTag = "cloud-nimbus"
	ress.CloudRegion = "catbus"
	ress.Qualifier = "prod"
	ress.Life = "alive"

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CreateModel", args, result).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	newModel, err := client.CreateModel(
		c.Context(),
		"new-model",
		"prod",
		"nimbus",
		"catbus",
		names.CloudCredentialTag{},
		map[string]interface{}{"abc": 123},
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(newModel, tc.DeepEquals, base.ModelInfo{
		Name:           "dowhatimean",
		Type:           model.IAAS,
		UUID:           "youyoueyedee",
		ControllerUUID: "youyoueyedeetoo",
		ProviderType:   "C-123",
		Cloud:          "nimbus",
		CloudRegion:    "catbus",
		Qualifier:      "prod",
		Life:           "alive",
		Status: base.Status{
			Data: make(map[string]interface{}),
		},
		Users:    []base.UserInfo{},
		Machines: []base.Machine{},
	})
}

func (s *modelmanagerSuite) TestListModelsBadUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ListModels(c.Context(), "not a user")
	c.Assert(err, tc.ErrorMatches, `invalid user name "not a user"`)
}

func (s *modelmanagerSuite) TestListModels(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	lastConnection := time.Now()
	args := params.Entity{Tag: "user-user@remote"}

	result := new(params.UserModelList)
	ress := params.UserModelList{
		UserModels: []params.UserModel{{
			Model: params.Model{
				Name:      "yo",
				UUID:      "wei",
				Type:      "caas",
				Qualifier: "prod",
			},
			LastConnection: &lastConnection,
		}, {
			Model: params.Model{
				Name:      "sup",
				UUID:      "hazzagarn",
				Type:      "iaas",
				Qualifier: "staging",
			},
		}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListModels", args, result).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	models, err := client.ListModels(c.Context(), "user@remote")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, []base.UserModel{{
		Name:           "yo",
		UUID:           "wei",
		Type:           model.CAAS,
		Qualifier:      "prod",
		LastConnection: &lastConnection,
	}, {
		Name:      "sup",
		UUID:      "hazzagarn",
		Type:      model.IAAS,
		Qualifier: "staging",
	}})
}

func (s *modelmanagerSuite) testDestroyModel(c *tc.C, destroyStorage, force *bool, maxWait *time.Duration, timeout time.Duration) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.DestroyModelsParams{
		Models: []params.DestroyModelParams{{
			ModelTag:       coretesting.ModelTag.String(),
			DestroyStorage: destroyStorage,
			Force:          force,
			MaxWait:        maxWait,
			Timeout:        &timeout,
		}},
	}

	result := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyModels", args, result).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.DestroyModel(c.Context(), coretesting.ModelTag, destroyStorage, force, maxWait, &timeout)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelmanagerSuite) TestDestroyModel(c *tc.C) {
	true_ := true
	false_ := false
	defaultMin := 1 * time.Minute
	s.testDestroyModel(c, nil, nil, nil, time.Minute)
	s.testDestroyModel(c, nil, &true_, nil, time.Minute)
	s.testDestroyModel(c, nil, &true_, &defaultMin, time.Minute)
	s.testDestroyModel(c, nil, &false_, nil, time.Minute)
	s.testDestroyModel(c, &true_, nil, nil, time.Minute)
	s.testDestroyModel(c, &true_, &false_, nil, time.Minute)
	s.testDestroyModel(c, &true_, &true_, &defaultMin, time.Minute)
	s.testDestroyModel(c, &false_, nil, nil, time.Minute)
	s.testDestroyModel(c, &false_, &false_, nil, time.Minute)
	s.testDestroyModel(c, &false_, &true_, &defaultMin, time.Minute)
}

func (s *modelmanagerSuite) TestModelDefaults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("aws").String()}},
	}

	res := new(params.ModelDefaultsResults)
	ress := params.ModelDefaultsResults{
		Results: []params.ModelDefaultsResult{{Config: map[string]params.ModelDefaults{
			"foo": {"bar", "model", []params.RegionDefaults{{
				"dummy-region",
				"dummy-value"}}},
		}}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelDefaultsForClouds", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	result, err := client.ModelDefaults(c.Context(), "aws")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result, tc.DeepEquals, config.ModelDefaultAttributes{
		"foo": {"bar", "model", []config.RegionDefaultValue{{
			"dummy-region",
			"dummy-value"}}},
	})
}

func (s *modelmanagerSuite) TestSetModelDefaults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			CloudTag:    "cloud-mycloud",
			CloudRegion: "region",
			Config: map[string]interface{}{
				"some-name":  "value",
				"other-name": true,
			},
		}}}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetModelDefaults", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.SetModelDefaults(c.Context(), "mycloud", "region", map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelmanagerSuite) TestUnsetModelDefaults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			CloudTag:    "cloud-mycloud",
			CloudRegion: "region",
			Keys:        []string{"foo", "bar"},
		}}}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UnsetModelDefaults", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.UnsetModelDefaults(c.Context(), "mycloud", "region", "foo", "bar")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelmanagerSuite) TestModelStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: coretesting.ModelTag.String()},
			{Tag: coretesting.ModelTag.String()},
		},
	}

	res := new(params.ModelStatusResults)
	ress := params.ModelStatusResults{
		Results: []params.ModelStatus{
			{
				ModelTag:           coretesting.ModelTag.String(),
				Qualifier:          "prod",
				ApplicationCount:   3,
				HostedMachineCount: 2,
				Life:               "alive",
				Machines: []params.ModelMachineInfo{{
					Id:         "0",
					InstanceId: "inst-ance",
					Status:     "pending",
				}},
			},
			{
				Error: apiservererrors.ServerError(errors.New("model error")),
			},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", args, res).SetArg(3, ress).Return(nil)
	client := common.NewModelStatusAPI(mockFacadeCaller, false)

	results, err := client.ModelStatus(c.Context(), coretesting.ModelTag, coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results[0], tc.DeepEquals, base.ModelStatus{
		UUID:               coretesting.ModelTag.Id(),
		TotalMachineCount:  1,
		HostedMachineCount: 2,
		ApplicationCount:   3,
		Qualifier:          "prod",
		Life:               life.Alive,
		Machines:           []base.Machine{{Id: "0", InstanceId: "inst-ance", Status: "pending"}},
	})
	c.Assert(results[1].Error, tc.ErrorMatches, "model error")
}

func (s *modelmanagerSuite) TestModelStatusEmpty(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{},
	}

	res := new(params.ModelStatusResults)
	ress := params.ModelStatusResults{}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", args, res).SetArg(3, ress).Return(nil)
	client := common.NewModelStatusAPI(mockFacadeCaller, false)

	results, err := client.ModelStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []base.ModelStatus{})
}

func (s *modelmanagerSuite) TestModelStatusError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: coretesting.ModelTag.String()},
			{Tag: coretesting.ModelTag.String()},
		},
	}

	res := new(params.ModelStatusResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", args, res).Return(errors.New("model error"))
	client := common.NewModelStatusAPI(mockFacadeCaller, false)
	out, err := client.ModelStatus(c.Context(), coretesting.ModelTag, coretesting.ModelTag)
	c.Assert(err, tc.ErrorMatches, "model error")
	c.Assert(out, tc.IsNil)
}

func createModelSummary() *params.ModelSummary {
	return &params.ModelSummary{
		Name:               "name",
		UUID:               "uuid",
		Type:               "iaas",
		ControllerUUID:     "controllerUUID",
		ProviderType:       "aws",
		CloudTag:           "cloud-aws",
		CloudRegion:        "us-east-1",
		CloudCredentialTag: "cloudcred-foo_bob_one",
		Qualifier:          "prod",
		Life:               life.Alive,
		Status:             params.EntityStatus{Status: status.Status("active")},
		UserAccess:         params.ModelAdminAccess,
		Counts:             []params.ModelEntityCount{},
	}
}

func (s *modelmanagerSuite) TestListModelSummaries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	userTag := names.NewUserTag("commander")
	testModelInfo := createModelSummary()

	args := params.ModelSummariesRequest{
		UserTag: userTag.String(),
		All:     true,
	}

	res := new(params.ModelSummaryResults)
	ress := params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{Result: testModelInfo},
			{Error: apiservererrors.ServerError(errors.New("model error"))},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListModelSummaries", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ListModelSummaries(c.Context(), userTag.Id(), true)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 2)
	c.Assert(results[0], tc.DeepEquals, base.UserModelSummary{Name: testModelInfo.Name,
		UUID:            testModelInfo.UUID,
		Type:            model.IAAS,
		ControllerUUID:  testModelInfo.ControllerUUID,
		ProviderType:    testModelInfo.ProviderType,
		Cloud:           "aws",
		CloudRegion:     "us-east-1",
		CloudCredential: "foo/bob/one",
		Qualifier:       "prod",
		Life:            "alive",
		Status: base.Status{
			Status: status.Active,
			Data:   map[string]interface{}{},
		},
		ModelUserAccess: "admin",
		Counts:          []base.EntityCount{},
	})
	c.Assert(errors.Cause(results[1].Error), tc.ErrorMatches, "model error")
}

func (s *modelmanagerSuite) TestListModelSummariesParsingErrors(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	badCloudInfo := createModelSummary()
	badCloudInfo.CloudTag = "not-cloud"

	badCredentialsInfo := createModelSummary()
	badCredentialsInfo.CloudCredentialTag = "not-credential"

	args := params.ModelSummariesRequest{
		UserTag: "user-commander",
		All:     true,
	}

	res := new(params.ModelSummaryResults)
	ress := params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{Result: badCloudInfo},
			{Result: badCredentialsInfo},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListModelSummaries", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	results, err := client.ListModelSummaries(c.Context(), "commander", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Assert(results[0].Error, tc.ErrorMatches, `parsing model cloud tag: "not-cloud" is not a valid tag`)
	c.Assert(results[1].Error, tc.ErrorMatches, `parsing model cloud credential tag: "not-credential" is not a valid tag`)
}

func (s *modelmanagerSuite) TestListModelSummariesInvalidUserIn(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	out, err := client.ListModelSummaries(c.Context(), "++)captain", false)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`invalid user name "++)captain"`))
	c.Assert(out, tc.IsNil)
}

func (s *modelmanagerSuite) TestListModelSummariesServerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ModelSummariesRequest{
		UserTag: "user-captain",
		All:     false,
	}

	res := new(params.ModelSummaryResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListModelSummaries", args, res).Return(errors.New("captain, error"))
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	out, err := client.ListModelSummaries(c.Context(), "captain", false)
	c.Assert(err, tc.ErrorMatches, "captain, error")
	c.Assert(out, tc.IsNil)
}

func (s *modelmanagerSuite) TestChangeModelCredential(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar")
	args := params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: coretesting.ModelTag.String(), CloudCredentialTag: credentialTag.String()},
		},
	}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ChangeModelCredential", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.ChangeModelCredential(c.Context(), coretesting.ModelTag, credentialTag)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelmanagerSuite) TestChangeModelCredentialManyResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar")

	args := params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: coretesting.ModelTag.String(), CloudCredentialTag: credentialTag.String()},
		},
	}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ChangeModelCredential", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.ChangeModelCredential(c.Context(), coretesting.ModelTag, credentialTag)
	c.Assert(err, tc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *modelmanagerSuite) TestChangeModelCredentialCallFailed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar")
	args := params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: coretesting.ModelTag.String(), CloudCredentialTag: credentialTag.String()},
		},
	}

	res := new(params.ErrorResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ChangeModelCredential", args, res).Return(errors.New("failed call"))
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)
	err := client.ChangeModelCredential(c.Context(), coretesting.ModelTag, credentialTag)
	c.Assert(err, tc.ErrorMatches, `failed call`)
}

func (s *modelmanagerSuite) TestChangeModelCredentialUpdateFailed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar")
	args := params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: coretesting.ModelTag.String(), CloudCredentialTag: credentialTag.String()},
		},
	}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{Error: apiservererrors.ServerError(errors.New("update error"))}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ChangeModelCredential", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	err := client.ChangeModelCredential(c.Context(), coretesting.ModelTag, credentialTag)
	c.Assert(err, tc.ErrorMatches, `update error`)
}

type dumpModelSuite struct {
	coretesting.BaseSuite
}

func TestDumpModelSuite(t *testing.T) {
	tc.Run(t, &dumpModelSuite{})
}

func (s *dumpModelSuite) TestDumpModelDB(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expected := map[string]interface{}{
		"models": []map[string]interface{}{{
			"name": "admin",
			"uuid": "some-uuid",
		}},
		"machines": []map[string]interface{}{{
			"id":   "0",
			"life": 0,
		}},
	}
	args := params.Entities{[]params.Entity{{coretesting.ModelTag.String()}}}

	res := new(params.MapResults)
	ress := params.MapResults{Results: []params.MapResult{{
		Result: expected,
	}}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DumpModelsDB", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	out, err := client.DumpModelDB(c.Context(), coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, expected)
}

func (s *dumpModelSuite) TestDumpModelDBError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{[]params.Entity{{coretesting.ModelTag.String()}}}

	res := new(params.MapResults)
	ress := params.MapResults{Results: []params.MapResult{{
		Error: &params.Error{Message: "fake error"},
	}}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DumpModelsDB", args, res).SetArg(3, ress).Return(nil)
	client := modelmanager.NewClientFromCaller(mockFacadeCaller)

	out, err := client.DumpModelDB(c.Context(), coretesting.ModelTag)
	c.Assert(err, tc.ErrorMatches, "fake error")
	c.Assert(out, tc.IsNil)
}
