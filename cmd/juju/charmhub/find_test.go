// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
)

type findSuite struct {
	api *mocks.MockFindCommandAPI
}

var _ = gc.Suite(&findSuite{})

func (s *findSuite) TestInitNoArgs(c *gc.C) {
	// You can query the find api with no arguments.
	command := &findCommand{}
	err := command.Init([]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestInitSuccess(c *gc.C) {
	command := &findCommand{}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()
	command := &findCommand{api: s.api, query: "test"}
	ctx := commandContextForTest(c)
	err := command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockFindCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}

func (s *findSuite) expectFind() {
	s.api.EXPECT().Find("test").Return([]charmhub.FindResponse{}, nil)
}
