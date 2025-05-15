// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coresecrets "github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/mocks"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/testhelpers"
)

type deleteBackendSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&deleteBackendSuite{})

func (s *deleteBackendSuite) TestGetContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockSecretsState(ctrl)
	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Fail()
		return nil, nil
	}

	uri := coresecrets.NewURI()
	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)
	_, err := client.GetContent(c.Context(), uri, "", false, false)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *deleteBackendSuite) TestSaveContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockSecretsState(ctrl)
	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Fail()
		return nil, nil
	}

	uri := coresecrets.NewURI()
	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)
	_, err := client.SaveContent(c.Context(), uri, 666, coresecrets.NewSecretValue(nil))
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *deleteBackendSuite) TestDeleteExternalContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockSecretsState(ctrl)
	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Fail()
		return nil, nil
	}

	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)
	err := client.DeleteExternalContent(c.Context(), coresecrets.ValueRef{})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *deleteBackendSuite) TestGetBackend(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockSecretsState(ctrl)
	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Fail()
		return nil, nil
	}

	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)
	_, _, err := client.GetBackend(c.Context(), ptr("someid"), false)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *deleteBackendSuite) TestGetRevisionContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockSecretsState(ctrl)
	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Fail()
		return nil, nil
	}

	uri := coresecrets.NewURI()
	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)
	_, err := client.GetRevisionContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *deleteBackendSuite) TestDeleteContent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := mocks.NewMockSecretsBackend(ctrl)
	state := mocks.NewMockSecretsState(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "somebackend1",
			Configs: map[string]provider.ModelBackendConfig{
				backendID: {
					ControllerUUID: "controller-uuid2",
					ModelUUID:      "model-uuid2",
					ModelName:      "model2",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
				},
			},
		}, nil
	}

	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)

	uri := coresecrets.NewURI()
	state.EXPECT().GetSecretValue(uri, 666).Return(nil, &coresecrets.ValueRef{
		BackendID:  "somebackend1",
		RevisionID: "rev-id",
	}, nil)
	backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)

	err := client.DeleteContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deleteBackendSuite) TestDeleteContentDraining(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backend := mocks.NewMockSecretsBackend(ctrl)
	state := mocks.NewMockSecretsState(ctrl)

	backends := set.NewStrings("somebackend1", "somebackend2")
	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(backends.Contains(cfg.BackendType), tc.IsTrue)
		return backend, nil
	})

	count := 0
	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		activeID := "somebackend2"
		if count > 0 {
			activeID = backendID
		}
		count++
		return &provider.ModelBackendConfigInfo{
			ActiveID: activeID,
			Configs: map[string]provider.ModelBackendConfig{
				backendID: {
					ControllerUUID: "controller-uuid2",
					ModelUUID:      "model-uuid2",
					ModelName:      "model2",
					BackendConfig:  provider.BackendConfig{BackendType: "somebackend1"},
				},
			},
		}, nil
	}

	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)

	uri := coresecrets.NewURI()
	state.EXPECT().GetSecretValue(uri, 666).Return(nil, &coresecrets.ValueRef{
		BackendID:  "somebackend1",
		RevisionID: "rev-id",
	}, nil).Times(2)
	backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(secreterrors.SecretRevisionNotFound)
	backend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)

	err := client.DeleteContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deleteBackendSuite) TestDeleteInternalContentNoop(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockSecretsState(ctrl)

	backendConfigForDeleteGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Fail()
		return nil, nil
	}

	client := secrets.NewClientForContentDeletion(state, backendConfigForDeleteGetter)

	uri := coresecrets.NewURI()
	state.EXPECT().GetSecretValue(uri, 666).Return(coresecrets.NewSecretValue(nil), nil, nil)

	err := client.DeleteContent(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}
