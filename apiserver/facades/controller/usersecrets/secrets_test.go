// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	"context"
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
)

type userSecretsSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	secretService *mocks.MockSecretService
	stringWatcher *mocks.MockStringsWatcher

	facade *usersecrets.UserSecretsManager
}

var _ = gc.Suite(&userSecretsSuite{})

func (s *userSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)
	s.stringWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)

	s.authorizer.EXPECT().AuthController().Return(true)

	var err error
	s.facade, err = usersecrets.NewTestAPI(
		s.authorizer, s.resources,
		s.secretService,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *userSecretsSuite) TestWatchRevisionsToPrune(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretService.EXPECT().WatchUserSecretsRevisionsToPrune(gomock.Any()).Return(s.stringWatcher, nil)
	s.resources.EXPECT().Register(s.stringWatcher).Return("watcher-id")
	stringChan := make(chan []string, 1)
	stringChan <- []string{"1", "2", "3"}
	s.stringWatcher.EXPECT().Changes().Return(stringChan)

	result, err := s.facade.WatchRevisionsToPrune(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "watcher-id",
		Changes:          []string{"1", "2", "3"},
	})
}

func (s *userSecretsSuite) TestDeleteRevisionsAutoPruneEnabled(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{
		URI:       uri,
		AutoPrune: true,
	}, nil)
	s.secretService.EXPECT().DeleteUserSecret(gomock.Any(), uri, []int{666}, gomock.Any()).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, revisions []int, canDelete func(uri *coresecrets.URI) error) error {
			return canDelete(uri)
		},
	)

	results, err := s.facade.DeleteRevisions(
		context.Background(),
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
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{
		URI:       uri,
		AutoPrune: false,
	}, nil)
	s.secretService.EXPECT().DeleteUserSecret(gomock.Any(), uri, []int{666}, gomock.Any()).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, revisions []int, canDelete func(uri *coresecrets.URI) error) error {
			return canDelete(uri)
		},
	)

	results, err := s.facade.DeleteRevisions(
		context.Background(),
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
