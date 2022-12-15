// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/mocks"
	"github.com/juju/juju/secrets/provider"
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

	jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id2",
		Configs: map[string]provider.BackendConfig{
			"backend-id1": {BackendType: "somebackend1"},
			"backend-id2": {BackendType: "somebackend2"},
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

	jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id2",
		Configs: map[string]provider.BackendConfig{
			"backend-id1": {BackendType: "somebackend1"},
			"backend-id2": {BackendType: "somebackend2"},
		},
	}, nil)

	uri := coresecrets.NewURI()
	jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id1",
			RevisionID: "rev-id",
		},
	}, nil)
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
		jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id2",
			Configs: map[string]provider.BackendConfig{
				"backend-id1": {BackendType: "somebackend1"},
				"backend-id2": {BackendType: "somebackend2"},
			},
		}, nil),
		jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id1",
				RevisionID: "rev-id",
			},
		}, nil),

		// First not found - we try again with the active backend.
		backend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(nil, errors.NotFoundf("")),
		jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id2",
				RevisionID: "rev-id2",
			},
		}, nil),

		// Second not found - refresh backend config.
		backend.EXPECT().GetContent(gomock.Any(), "rev-id2").Return(nil, errors.NotFoundf("")),
		jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id3",
			Configs: map[string]provider.BackendConfig{
				"backend-id3": {BackendType: "somebackend3"},
			},
		}, nil),

		// Third time lucky.
		jujuapi.EXPECT().GetContentInfo(uri, "label", true, false).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id3",
				RevisionID: "rev-id3",
			},
		}, nil),
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
	jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id2",
		Configs: map[string]provider.BackendConfig{
			"backend-id1": {BackendType: "somebackend1"},
			"backend-id2": {BackendType: "somebackend2"},
		},
	}, nil)
	jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id2",
			RevisionID: "rev-id",
		},
	}, nil)

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
		jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id2",
			Configs: map[string]provider.BackendConfig{
				"backend-id1": {BackendType: "somebackend1"},
				"backend-id2": {BackendType: "somebackend2"},
			},
		}, nil),

		// First not found - we try again with the active backend.
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id1",
				RevisionID: "rev-id",
			},
		}, nil),
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(errors.NotFoundf("")),

		// Second not found - refresh backend config.
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id2",
				RevisionID: "rev-id2",
			},
		}, nil),
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id2").Return(errors.NotFoundf("")),
		jujuapi.EXPECT().GetSecretBackendConfig().Return(&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id3",
			Configs: map[string]provider.BackendConfig{
				"backend-id3": {BackendType: "somebackend3"},
			},
		}, nil),

		// Third time lucky.
		jujuapi.EXPECT().GetRevisionContentInfo(uri, 666, true).Return(&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id3",
				RevisionID: "rev-id3",
			},
		}, nil),
		backend.EXPECT().DeleteContent(gomock.Any(), "rev-id3").Return(nil),
	)

	err = client.DeleteContent(uri, 666)
	c.Assert(err, jc.ErrorIsNil)
}
