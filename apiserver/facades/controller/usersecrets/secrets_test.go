// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
)

type userSecretsSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources
	authTag    names.Tag

	state         *mocks.MockSecretsState
	stringWatcher *mocks.MockStringsWatcher

	provider *mocks.MockSecretBackendProvider
	backend  *mocks.MockSecretsBackend

	facade *usersecrets.UserSecretsManager
}

var _ = gc.Suite(&userSecretsSuite{})

func (s *userSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)
	s.authTag = names.NewUserTag("foo")
	s.state = mocks.NewMockSecretsState(ctrl)
	s.stringWatcher = mocks.NewMockStringsWatcher(ctrl)

	s.provider = mocks.NewMockSecretBackendProvider(ctrl)
	s.backend = mocks.NewMockSecretsBackend(ctrl)
	s.PatchValue(&commonsecrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return s.provider, nil })

	s.authorizer.EXPECT().AuthController().Return(true)

	var err error
	s.facade, err = usersecrets.NewTestAPI(
		s.authorizer, s.resources, s.authTag,
		coretesting.ControllerTag.Id(), coretesting.ModelTag.Id(), s.state,
		func() (*provider.ModelBackendConfigInfo, error) {
			return &provider.ModelBackendConfigInfo{
				ActiveID: "backend-id",
				Configs: map[string]provider.ModelBackendConfig{
					"backend-id": {
						ControllerUUID: coretesting.ControllerTag.Id(),
						ModelUUID:      coretesting.ModelTag.Id(),
						ModelName:      "some-model",
						BackendConfig: provider.BackendConfig{
							BackendType: "active-type",
							Config:      map[string]interface{}{"foo": "active-type"},
						},
					},
				},
			}, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *userSecretsSuite) TestWatchRevisionsToPrune(c *gc.C) {
	defer s.setup(c).Finish()

	s.state.EXPECT().WatchRevisionsToPrune([]names.Tag{names.NewModelTag(coretesting.ModelTag.Id())}).Return(s.stringWatcher, nil)
	s.resources.EXPECT().Register(s.stringWatcher).Return("watcher-id")
	stringChan := make(chan []string, 1)
	stringChan <- []string{"1", "2", "3"}
	s.stringWatcher.EXPECT().Changes().Return(stringChan)

	result, err := s.facade.WatchRevisionsToPrune()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "watcher-id",
		Changes:          []string{"1", "2", "3"},
	})
}

func (s *userSecretsSuite) TestDeleteRevisionsAutoPruneEnabled(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		URI: uri, OwnerTag: coretesting.ModelTag.String(),
		AutoPrune: true,
	}, nil).Times(2)
	s.state.EXPECT().GetSecretRevision(uri, 666).Return(&coresecrets.SecretRevisionMetadata{
		Revision: 666,
		ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-666"},
	}, nil)
	s.state.EXPECT().DeleteSecret(uri, []int{666}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	cfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "some-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "active-type",
			Config:      map[string]interface{}{"foo": "active-type"},
		},
	}
	s.provider.EXPECT().NewBackend(cfg).Return(s.backend, nil)
	s.backend.EXPECT().DeleteContent(gomock.Any(), "rev-666").Return(nil)
	s.provider.EXPECT().CleanupSecrets(
		cfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-666")},
	).Return(nil)

	results, err := s.facade.DeleteRevisions(
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{
					URI:       uri.String(),
					Revisions: []int{666},
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *userSecretsSuite) TestDeleteRevisionsAutoPruneDisabled(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		URI: uri, OwnerTag: coretesting.ModelTag.String(),
		AutoPrune: false,
	}, nil).Times(2)

	results, err := s.facade.DeleteRevisions(
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{
					URI:       uri.String(),
					Revisions: []int{666},
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{
					Message: fmt.Sprintf("cannot delete non auto-prune secret %q", uri.String()),
				},
			},
		},
	})
}
