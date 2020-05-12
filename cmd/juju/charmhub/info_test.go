// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/charmhub/mocks"
)

type infoSuite struct {
	api *mocks.MockInfoCommandAPI
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) TestInitNoArgs(c *gc.C) {
	cmd := &infoCommand{}
	err := cmd.Init([]string{})
	c.Assert(err, gc.NotNil)
}

func (s *infoSuite) TestInitSuccess(c *gc.C) {
	cmd := &infoCommand{}
	err := cmd.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestRunNotImplemented(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	cmd := &infoCommand{api: s.api}
	err := cmd.Run(nil)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *infoSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockInfoCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}
