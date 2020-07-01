// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
)

type infoSuite struct {
	api *mocks.MockInfoCommandAPI
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) TestInitNoArgs(c *gc.C) {
	command := &infoCommand{}
	err := command.Init([]string{})
	c.Assert(err, gc.NotNil)
}

func (s *infoSuite) TestInitSuccess(c *gc.C) {
	command := &infoCommand{}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()
	command := &infoCommand{api: s.api, charmOrBundle: "test"}
	ctx := commandContextForTest(c)
	err := command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *infoSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockInfoCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}

func (s *infoSuite) expectInfo() {
	s.api.EXPECT().Info("test").Return(charmhub.InfoResponse{}, nil)
}

func commandContextForTest(c *gc.C) *cmd.Context {
	var stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr
	return ctx
}
