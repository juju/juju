// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coresecrets "github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/mocks"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/testhelpers"
)

type backendSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&backendSuite{})

func (s *backendSuite) TestSaveContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)

	jujuapi.EXPECT().GetSecretBackendConfig(gomock.Any(), nil).Return(&provider.ModelBackendConfigInfo{
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

	val, err := client.SaveContent(c.Context(), uri, 666, secretValue)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, coresecrets.ValueRef{
		BackendID:  "backend-id2",
		RevisionID: "rev-id",
	})
}

func (s *backendSuite) TestGetContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)

	uri := coresecrets.NewURI()
	jujuapi.EXPECT().GetContentInfo(gomock.Any(), uri, "label", true, false).Return(&secrets.ContentParams{
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

	val, err := client.GetContent(c.Context(), uri, "label", true, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, secretValue)
}

func (s *backendSuite) TestGetContentSecretDrained(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)

	uri := coresecrets.NewURI()
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	gomock.InOrder(
		jujuapi.EXPECT().GetContentInfo(gomock.Any(), uri, "label", true, false).Return(&secrets.ContentParams{
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
		backend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(nil, secreterrors.SecretRevisionNotFound),
		jujuapi.EXPECT().GetContentInfo(gomock.Any(), uri, "label", true, false).Return(&secrets.ContentParams{
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
		backend.EXPECT().GetContent(gomock.Any(), "rev-id2").Return(nil, secreterrors.SecretRevisionNotFound),

		// Third time lucky.
		jujuapi.EXPECT().GetContentInfo(gomock.Any(), uri, "label", true, false).Return(&secrets.ContentParams{
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

	val, err := client.GetContent(c.Context(), uri, "label", true, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, secretValue)
}

func (s *backendSuite) TestDeleteContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)

	uri := coresecrets.NewURI()
	jujuapi.EXPECT().GetRevisionContentInfo(gomock.Any(), uri, 666, true).Return(&secrets.ContentParams{
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

	err = client.DeleteContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *backendSuite) TestDeleteContentDrained(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)

	uri := coresecrets.NewURI()
	gomock.InOrder(
		jujuapi.EXPECT().GetRevisionContentInfo(gomock.Any(), uri, 666, true).Return(&secrets.ContentParams{
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
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(secreterrors.SecretRevisionNotFound),

		// Second not found - refresh backend config.
		jujuapi.EXPECT().GetRevisionContentInfo(gomock.Any(), uri, 666, true).Return(&secrets.ContentParams{
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
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id2").Return(secreterrors.SecretRevisionNotFound),

		// Third time lucky.
		jujuapi.EXPECT().GetRevisionContentInfo(gomock.Any(), uri, 666, true).Return(&secrets.ContentParams{
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

	err = client.DeleteContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *backendSuite) TestGetBackend(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	called := 0
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		called++
		if called == 1 {
			c.Assert(cfg.BackendType, tc.Equals, "somebackend2")
		} else {
			c.Assert(cfg.BackendType, tc.Equals, "somebackend1")
		}
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)
	backendID := "backend-id1"

	gomock.InOrder(
		jujuapi.EXPECT().GetSecretBackendConfig(gomock.Any(), nil).Return(&provider.ModelBackendConfigInfo{
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
		jujuapi.EXPECT().GetSecretBackendConfig(gomock.Any(), &backendID).Return(&provider.ModelBackendConfigInfo{
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
		jujuapi.EXPECT().GetBackendConfigForDrain(gomock.Any(), &backendID).Return(
			&provider.ModelBackendConfig{
				ControllerUUID: "controller-uuid1",
				ModelUUID:      "model-uuid1",
				ModelName:      "model1",
				BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
			}, "backend-id1", nil,
		),
	)
	result, activeBackendID, err := client.GetBackend(c.Context(), nil, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activeBackendID, tc.Equals, "backend-id2")
	c.Assert(result, tc.Equals, backend)

	result, activeBackendID, err = client.GetBackend(c.Context(), &backendID, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activeBackendID, tc.Equals, "backend-id2")
	c.Assert(result, tc.Equals, backend)

	result, activeBackendID, err = client.GetBackend(c.Context(), &backendID, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activeBackendID, tc.Equals, "backend-id1")
	c.Assert(result, tc.Equals, backend)
}

func (s *backendSuite) TestGetRevisionContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	jujuapi := mocks.NewMockJujuAPIClient(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2", "somebackend3")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	client, err := secrets.NewClient(jujuapi)
	c.Assert(err, tc.ErrorIsNil)

	uri := coresecrets.NewURI()
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	gomock.InOrder(
		jujuapi.EXPECT().GetRevisionContentInfo(gomock.Any(), uri, 666, false).Return(&secrets.ContentParams{
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
		jujuapi.EXPECT().GetSecretBackendConfig(gomock.Any(), ptr("backend-id2")).Return(&provider.ModelBackendConfigInfo{
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

	val, err := client.GetRevisionContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.Equals, secretValue)
}

func ptr[T any](v T) *T {
	return &v
}
