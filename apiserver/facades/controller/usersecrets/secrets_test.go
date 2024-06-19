// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	"context"

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

	secretService *mocks.MockSecretService
	watcher       *mocks.MockStringsWatcher

	facade          *usersecrets.UserSecretsManager
	watcherRegistry *facademocks.MockWatcherRegistry
}

var _ = gc.Suite(&userSecretsSuite{})

func (s *userSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.watcher = mocks.NewMockStringsWatcher(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.authorizer.EXPECT().AuthController().Return(true)

	var err error
	s.facade, err = usersecrets.NewTestAPI(s.authorizer, s.watcherRegistry, s.secretService)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *userSecretsSuite) TestWatchRevisionsToPrune(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretService.EXPECT().WatchObsoleteUserSecretsToPrune(gomock.Any()).Return(s.watcher, nil)
	ch := make(chan []string, 1)
	ch <- []string{"secret-id/1"}
	s.watcher.EXPECT().Changes().Return(ch)

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("watcher-id", nil)

	result, err := s.facade.WatchRevisionsToPrune(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "watcher-id",
		Changes:          []string{"secret-id/1"},
	})
}

func (s *userSecretsSuite) TestDeleteRevisionsAutoPruneEnabled(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()

	s.secretService.EXPECT().DeleteObsoleteUserSecrets(gomock.Any(), uri, []int{1, 2}).Return(nil)

	err := s.facade.DeleteObsoleteUserSecrets(context.Background(),
		params.DeleteSecretArg{URI: uri.String(), Revisions: []int{1, 2}},
	)
	c.Assert(err, jc.ErrorIsNil)
}
