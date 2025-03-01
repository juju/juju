// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/juju/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/sshserver"
	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&sshserverSuite{})

type sshserverSuite struct {
	testing.IsolationSuite
}

func (s *sshserverSuite) TestAuth(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := NewMockContext(ctrl)
	authorizer := NewMockAuthorizer(ctrl)

	ctx.EXPECT().Auth().Return(authorizer)
	authorizer.EXPECT().AuthController().Return(false)

	_, err := sshserver.NewExternalFacade(ctx)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *sshserverSuite) TestControllerConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := NewMockContext(ctrl)
	backend := NewMockBackend(ctrl)

	ctx.EXPECT().Resources().Times(1)
	backend.EXPECT().ControllerConfig().Return(
		controller.Config{"hi": "bye"},
		nil,
	)

	f := sshserver.NewFacade(ctx, backend)

	cfg, err := f.ControllerConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.DeepEquals, params.ControllerConfigResult{Config: params.ControllerConfig{"hi": "bye"}})
}

func (s *sshserverSuite) TestWatchControllerConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := NewMockContext(ctrl)
	backend := NewMockBackend(ctrl)
	resources := NewMockResources(ctrl)
	watcher := workertest.NewFakeWatcher(1, 0)
	watcher.Ping() // Send some changes

	ctx.EXPECT().Resources().Return(resources)
	backend.EXPECT().WatchControllerConfig().Return(watcher)
	resources.EXPECT().Register(watcher).Return("id")

	f := sshserver.NewFacade(ctx, backend)

	result, err := f.WatchControllerConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(result.NotifyWatcherId, gc.Equals, "id")

	// Now we close the channel expecting err
	watcher.Close()
	backend.EXPECT().WatchControllerConfig().Return(watcher)

	_, err = f.WatchControllerConfig()
	c.Assert(err, gc.ErrorMatches, "An error")
}

func (s *sshserverSuite) TestSSHServerHostKey(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := NewMockContext(ctrl)
	backend := NewMockBackend(ctrl)

	ctx.EXPECT().Resources().Times(1)
	backend.EXPECT().SSHServerHostKey().Return("hostkey", nil)

	f := sshserver.NewFacade(ctx, backend)

	key, err := f.SSHServerHostKey()
	c.Assert(err, gc.IsNil)
	c.Assert(key, gc.Equals, params.StringResult{Result: "hostkey"})
}
