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
	"github.com/juju/juju/rpc/params"
)

type userSecretsSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	secretService *mocks.MockSecretService
	watcher       *mocks.MockNotifyWatcher

	facade *usersecrets.UserSecretsManager
}

var _ = gc.Suite(&userSecretsSuite{})

func (s *userSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)
	s.watcher = mocks.NewMockNotifyWatcher(ctrl)
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

	s.secretService.EXPECT().WatchObsoleteUserSecrets(gomock.Any()).Return(s.watcher, nil)
	s.resources.EXPECT().Register(s.watcher).Return("watcher-id")
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	s.watcher.EXPECT().Changes().Return(ch)

	result, err := s.facade.WatchRevisionsToPrune(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "watcher-id",
	})
}

func (s *userSecretsSuite) TestDeleteRevisionsAutoPruneEnabled(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretService.EXPECT().DeleteObsoleteUserSecrets(gomock.Any()).Return(nil)

	err := s.facade.DeleteObsoleteUserSecrets(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}
