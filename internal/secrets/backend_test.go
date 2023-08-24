// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/mocks"
	"github.com/juju/juju/internal/secrets/provider"
)

type backendSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&backendSuite{})

func (s *backendSuite) TestSaveContent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)

	jujuapi.EXPECT().GetSecretBackendConfig(nil).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id2",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id1": {
				ControllerUUID: "controller-uuid1",
				ModelUUID:      "model-uuid1",
				ModelName:      "model1",
				BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
			},
			"backend-id2": {
				ControllerUUID: "controller-uuid2",
				ModelUUID:      "model-uuid2",
				ModelName:      "model2",
				BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
			},
		},
	}, nil)

	uri := coresecrets.NewURI()
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	backend.EXPECT().SaveContent(gomock.Any(), uri, 666, secretValue).Return("rev-id", nil)

	val, err := client.SaveContent(uri, 666, secretValue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, coresecrets.ValueRef{
		BackendID:  "backend-id2",
		RevisionID: "rev-id",
	})
}

func (s *backendSuite) TestGetContent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)

	uri := coresecrets.NewURI()
	jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id1",
			RevisionID: "rev-id",
		},
	}, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid1",
		ModelUUID:      "model-uuid1",
		ModelName:      "model1",
		BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
	}, false, nil)
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	backend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(secretValue, nil)

	val, err := client.GetContent(uri, "label", true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, secretValue)
}

func (s *backendSuite) TestGetContentSecretDrained(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)

	uri := coresecrets.NewURI()
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	gomock.InOrder(
		jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id1",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid1",
			ModelUUID:      "model-uuid1",
			ModelName:      "model1",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
		}, true, nil),

		// First not found - we try again with the active backend.
		backend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(nil, errors.NotFoundf("")),
		jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id2",
				RevisionID: "rev-id2",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid2",
			ModelUUID:      "model-uuid2",
			ModelName:      "model2",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
		}, true, nil),

		// Second not found - refresh backend config.
		backend.EXPECT().GetContent(gomock.Any(), "rev-id2").Return(nil, errors.NotFoundf("")),

		// Third time lucky.
		jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id3",
				RevisionID: "rev-id3",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid3",
			ModelUUID:      "model-uuid3",
			ModelName:      "model3",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend3"},
		}, false, nil),
		backend.EXPECT().GetContent(gomock.Any(), "rev-id3").Return(secretValue, nil),
	)

	val, err := client.GetContent(uri, "label", true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, secretValue)
}

func (s *backendSuite) TestDeleteContent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)

	uri := coresecrets.NewURI()
	jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id2",
			RevisionID: "rev-id",
		},
	}, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid2",
		ModelUUID:      "model-uuid2",
		ModelName:      "model2",
		BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
	}, false, nil)

	backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)

	err = client.DeleteContent(uri, 666)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *backendSuite) TestDeleteContentDrained(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)

	uri := coresecrets.NewURI()
	gomock.InOrder(
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id1",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid1",
			ModelUUID:      "model-uuid1",
			ModelName:      "model1",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
		}, true, nil),
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(errors.NotFoundf("")),

		// Second not found - refresh backend config.
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id2",
				RevisionID: "rev-id2",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid2",
			ModelUUID:      "model-uuid2",
			ModelName:      "model2",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
		}, true, nil),
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id2").Return(errors.NotFoundf("")),

		// Third time lucky.
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id3",
				RevisionID: "rev-id3",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid2",
			ModelUUID:      "model-uuid2",
			ModelName:      "model2",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
		}, false, nil),
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id3").Return(nil),
	)

	err = client.DeleteContent(uri, 666)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *backendSuite) TestGetBackend(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	called := 0
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		called++
		if called == 1 {
			c.Assert(cfg.BackendType, gc.Equals, "somebackend2")
		} else {
			c.Assert(cfg.BackendType, gc.Equals, "somebackend1")
		}
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)
	backendID := "backend-id1"

	gomock.InOrder(
		jujuapi.EXPECT().GetSecretBackendConfig(nil).Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id2",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id1": {
					ControllerUUID: "controller-uuid1",
					ModelUUID:      "model-uuid1",
					ModelName:      "model1",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
				},
				"backend-id2": {
					ControllerUUID: "controller-uuid2",
					ModelUUID:      "model-uuid2",
					ModelName:      "model2",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
				},
			},
		}, nil),
		jujuapi.EXPECT().GetSecretBackendConfig(&backendID).Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id2",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id1": {
					ControllerUUID: "controller-uuid1",
					ModelUUID:      "model-uuid1",
					ModelName:      "model1",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
				},
				"backend-id2": {
					ControllerUUID: "controller-uuid2",
					ModelUUID:      "model-uuid2",
					ModelName:      "model2",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
				},
			},
		}, nil),
		jujuapi.EXPECT().GetBackendConfigForDrain(&backendID).Return(
			&provider.ModelBackendConfig{
				ControllerUUID: "controller-uuid1",
				ModelUUID:      "model-uuid1",
				ModelName:      "model1",
				BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
			}, "backend-id1", nil,
		),
	)
	result, activeBackendID, err := client.GetBackend(nil, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeBackendID, gc.Equals, "backend-id2")
	c.Assert(result, gc.Equals, backend)

	result, activeBackendID, err = client.GetBackend(&backendID, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeBackendID, gc.Equals, "backend-id2")
	c.Assert(result, gc.Equals, backend)

	result, activeBackendID, err = client.GetBackend(&backendID, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeBackendID, gc.Equals, "backend-id1")
	c.Assert(result, gc.Equals, backend)
}

func (s *backendSuite) TestGetRevisionContent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), jc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, jc.ErrorIsNil)

	uri := coresecrets.NewURI()
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	gomock.InOrder(
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id2",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: "controller-uuid1",
			ModelUUID:      "model-uuid2",
			ModelName:      "model2",
			BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
		}, false, nil),
		jujuapi.EXPECT().GetSecretBackendConfig(ptr("backend-id2")).Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id2",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id1": {
					ControllerUUID: "controller-uuid1",
					ModelUUID:      "model-uuid1",
					ModelName:      "model1",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
				},
				"backend-id2": {
					ControllerUUID: "controller-uuid1",
					ModelUUID:      "model-uuid2",
					ModelName:      "model2",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend2"},
				},
			},
		}, nil),
		backend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(secretValue, nil),
	)

	val, err := client.GetRevisionContent(uri, 666)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, gc.Equals, secretValue)
}

func ptr[T any](v T) *T {
	return &v
}
