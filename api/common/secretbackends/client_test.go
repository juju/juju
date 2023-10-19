// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common/secretbackends"
	"github.com/juju/juju/api/common/secretbackends/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SecretsSuite{})

type SecretsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretsSuite) TestNewClient(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)
	client := secretbackends.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestGetSecretBackendConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(
		"GetSecretBackendConfigs",
		params.SecretBackendArgs{BackendIDs: []string{"active-id"}},
		gomock.Any(),
	).SetArg(
		2, params.SecretBackendConfigResults{
			ActiveID: "active-id",
			Results: map[string]params.SecretBackendConfigResult{
				"active-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Config: params.SecretBackendConfig{
						BackendType: "controller",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	result, err := client.GetSecretBackendConfig(ptr("active-id"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "active-id",
		Configs: map[string]provider.ModelBackendConfig{
			"active-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "controller",
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	})
}

func (s *SecretsSuite) TestGetBackendConfigForDraing(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(
		"GetSecretBackendConfigs",
		params.SecretBackendArgs{ForDrain: true, BackendIDs: []string{"active-id"}},
		gomock.Any(),
	).SetArg(
		2, params.SecretBackendConfigResults{
			ActiveID: "active-id",
			Results: map[string]params.SecretBackendConfigResult{
				"active-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Config: params.SecretBackendConfig{
						BackendType: "controller",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	result, activeID, err := client.GetBackendConfigForDrain(ptr("active-id"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "controller",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(activeID, gc.Equals, "active-id")
}

func (s *SecretsSuite) TestGetContentInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"GetSecretContentInfo",
		params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(uri, "label", true, true)
	c.Assert(err, jc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, jc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, gc.IsNil)
	c.Assert(draining, jc.IsFalse)
}

func (s *SecretsSuite) TestGetContentInfoExternal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"GetSecretContentInfo",
		params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: "controller-uuid",
					ModelUUID:      "model-uuid",
					ModelName:      "model",
					Draining:       true,
					Config: params.SecretBackendConfig{
						BackendType: "some-backend",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(uri, "label", true, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(content, jc.DeepEquals, &secrets.ContentParams{ValueRef: &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	c.Assert(backendConfig, jc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
		ModelName:      "model",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(draining, jc.IsTrue)
}

func (s *SecretsSuite) TestGetContentInfoLabelArgOnly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(
		"GetSecretContentInfo",
		params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(nil, "label", true, true)
	c.Assert(err, jc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, jc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, gc.IsNil)
	c.Assert(draining, jc.IsFalse)
}

func (s *SecretsSuite) TestGetContentInfoError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"GetSecretContentInfo",
		params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Refresh: true,
				Peek:    true,
			}},
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, _, err := client.GetContentInfo(uri, "", true, true)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(content, gc.IsNil)
	c.Assert(backendConfig, gc.IsNil)
}

func (s *SecretsSuite) TestGetRevisionContentInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"GetSecretRevisionContentInfo",
		params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetRevisionContentInfo(uri, 666, true)
	c.Assert(err, jc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, jc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, gc.IsNil)
	c.Assert(draining, jc.IsFalse)
}

func (s *SecretsSuite) TestGetRevisionContentInfoExternal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"GetSecretRevisionContentInfo",
		params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: "controller-uuid",
					ModelUUID:      "model-uuid",
					ModelName:      "model",
					Draining:       true,
					Config: params.SecretBackendConfig{
						BackendType: "some-backend",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetRevisionContentInfo(uri, 666, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(content, jc.DeepEquals, &secrets.ContentParams{ValueRef: &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	c.Assert(backendConfig, jc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
		ModelName:      "model",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(draining, jc.IsTrue)
}

func (s *SecretsSuite) TestGetRevisionContentInfoError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"GetSecretRevisionContentInfo",
		params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		},
		gomock.Any(),
	).SetArg(
		2, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	config, backendConfig, _, err := client.GetRevisionContentInfo(uri, 666, true)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(config, gc.IsNil)
	c.Assert(backendConfig, gc.IsNil)
}
