// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets"
	"github.com/juju/juju/apiserver/facades/controller/usersecrets/mocks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type userSecretsSuite struct {
	testhelpers.IsolationSuite

	authorizer *facademocks.MockAuthorizer

	secretService *mocks.MockSecretService
	watcher       *mocks.MockNotifyWatcher

	facade          *usersecrets.UserSecretsManager
	watcherRegistry *facademocks.MockWatcherRegistry
}

func TestUserSecretsSuite(t *testing.T) {
	tc.Run(t, &userSecretsSuite{})
}

func (s *userSecretsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.watcher = mocks.NewMockNotifyWatcher(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.authorizer.EXPECT().AuthController().Return(true)

	var err error
	s.facade, err = usersecrets.NewTestAPI(s.authorizer, s.watcherRegistry, s.secretService)
	c.Assert(err, tc.ErrorIsNil)
	return ctrl
}

func (s *userSecretsSuite) TestWatchRevisionsToPrune(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretService.EXPECT().WatchObsoleteUserSecretsToPrune(gomock.Any()).Return(s.watcher, nil)
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	s.watcher.EXPECT().Changes().Return(ch)

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("watcher-id", nil)

	result, err := s.facade.WatchRevisionsToPrune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "watcher-id",
	})
}

func (s *userSecretsSuite) TestDeleteRevisionsAutoPruneEnabled(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretService.EXPECT().DeleteObsoleteUserSecretRevisions(gomock.Any()).Return(nil)
	err := s.facade.DeleteObsoleteUserSecretRevisions(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
