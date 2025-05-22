// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/common/secretbackends"
	"github.com/juju/juju/api/common/secretbackends/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestSecretsSuite(t *testing.T) {
	tc.Run(t, &SecretsSuite{})
}

type SecretsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretsSuite) TestNewClient(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)
	client := secretbackends.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestGetSecretBackendConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
		"GetSecretBackendConfigs",
		params.SecretBackendArgs{BackendIDs: []string{"active-id"}},
		gomock.Any(),
	).SetArg(
		3, params.SecretBackendConfigResults{
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
	result, err := client.GetSecretBackendConfig(c.Context(), ptr("active-id"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &provider.ModelBackendConfigInfo{
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

func (s *SecretsSuite) TestGetBackendConfigForDraing(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
		"GetSecretBackendConfigs",
		params.SecretBackendArgs{ForDrain: true, BackendIDs: []string{"active-id"}},
		gomock.Any(),
	).SetArg(
		3, params.SecretBackendConfigResults{
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
	result, activeID, err := client.GetBackendConfigForDrain(c.Context(), ptr("active-id"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "controller",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(activeID, tc.Equals, "active-id")
}

func (s *SecretsSuite) TestGetContentInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
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
		3, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(c.Context(), uri, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, tc.IsNil)
	c.Assert(draining, tc.IsFalse)
}

func (s *SecretsSuite) TestGetContentInfoExternal(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
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
		3, params.SecretContentResults{
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
	content, backendConfig, draining, err := client.GetContentInfo(c.Context(), uri, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{ValueRef: &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	c.Assert(backendConfig, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
		ModelName:      "model",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(draining, tc.IsTrue)
}

func (s *SecretsSuite) TestGetContentInfoLabelArgOnly(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
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
		3, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(c.Context(), nil, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, tc.IsNil)
	c.Assert(draining, tc.IsFalse)
}

func (s *SecretsSuite) TestGetContentInfoError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
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
		3, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, _, err := client.GetContentInfo(c.Context(), uri, "", true, true)
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(content, tc.IsNil)
	c.Assert(backendConfig, tc.IsNil)
}

func (s *SecretsSuite) TestGetRevisionContentInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
		"GetSecretRevisionContentInfo",
		params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		},
		gomock.Any(),
	).SetArg(
		3, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetRevisionContentInfo(c.Context(), uri, 666, true)
	c.Assert(err, tc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, tc.IsNil)
	c.Assert(draining, tc.IsFalse)
}

func (s *SecretsSuite) TestGetRevisionContentInfoExternal(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
		"GetSecretRevisionContentInfo",
		params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		},
		gomock.Any(),
	).SetArg(
		3, params.SecretContentResults{
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
	content, backendConfig, draining, err := client.GetRevisionContentInfo(c.Context(), uri, 666, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{ValueRef: &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	c.Assert(backendConfig, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
		ModelName:      "model",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(draining, tc.IsTrue)
}

func (s *SecretsSuite) TestGetRevisionContentInfoError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
		"GetSecretRevisionContentInfo",
		params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		},
		gomock.Any(),
	).SetArg(
		3, params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		},
	).Return(nil)

	client := secretbackends.NewClient(apiCaller)
	config, backendConfig, _, err := client.GetRevisionContentInfo(c.Context(), uri, 666, true)
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(config, tc.IsNil)
	c.Assert(backendConfig, tc.IsNil)
}
