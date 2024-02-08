// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type UnexposeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UnexposeSuite{})

func runUnexpose(c *gc.C, api ApplicationExposeAPI, args ...string) error {
	unexposeCmd := &unexposeCommand{api: api}
	unexposeCmd.SetClientStore(jujuclienttesting.MinimalStore())

	_, err := cmdtesting.RunCommand(c, modelcmd.WrapBase(unexposeCmd), args...)
	return err
}

func (s *UnexposeSuite) TestUnexpose(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Unexpose("some-application-name", []string(nil)).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runUnexpose(c, api, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnexposeSuite) TestUnexposeEndpoints(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Unexpose("some-application-name", []string{"ep1", "ep2"}).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runUnexpose(c, api, "some-application-name", "--endpoints", "ep1,ep2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnexposeSuite) TestBlockUnexpose(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Unexpose("some-application-name", []string(nil)).Return(apiservererrors.OperationBlockedError("unexpose"))
	api.EXPECT().Close().Return(nil)

	err := runUnexpose(c, api, "some-application-name")
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), jc.IsTrue)
}
