// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/state/watcher/watchertest"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}

func (s *serviceSuite) TestUpdateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), "foo", "cirrus", "fred", cred).Return(nil)

	err := NewService(s.state, nil).UpdateCloudCredential(context.Background(), tag, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	two := cloud.NewNamedCredential("foobar", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().CloudCredentials(gomock.Any(), "fred", "cirrus").Return(map[string]cloud.Credential{
		"foo":    one,
		"foobar": two,
	}, nil)

	creds, err := NewService(s.state, nil).CloudCredentials(context.Background(), "fred", "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
		"foo":    one,
		"foobar": two,
	})
}

func (s *serviceSuite) TestCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().CloudCredential(gomock.Any(), "foo", "cirrus", "fred").Return(cred, nil)

	result, err := NewService(s.state, nil).CloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cred)
}

func (s *serviceSuite) TestRemoveCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), "foo", "cirrus", "fred").Return(nil)

	err := NewService(s.state, nil).RemoveCloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestAllCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().AllCloudCredentials(gomock.Any(), "fred").Return([]state.CloudCredential{{CloudName: "cirrus", Credential: cred}}, nil)

	result, err := NewService(s.state, nil).AllCloudCredentials(context.Background(), "fred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []CloudCredential{{CloudName: "cirrus", Credential: cred}})
}

func (s *serviceSuite) TestWatchCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewNotifyWatcher(nil)

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	s.state.EXPECT().WatchCredential(gomock.Any(), gomock.Any(), "foo", "cirrus", "fred").Return(nw, nil)

	w, err := NewService(s.state, s.watcherFactory).WatchCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}
