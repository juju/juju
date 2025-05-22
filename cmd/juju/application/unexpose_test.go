// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type UnexposeSuite struct {
	testing.BaseSuite
}

func TestUnexposeSuite(t *stdtesting.T) {
	tc.Run(t, &UnexposeSuite{})
}

func runUnexpose(c *tc.C, api ApplicationExposeAPI, args ...string) error {
	unexposeCmd := &unexposeCommand{api: api}
	unexposeCmd.SetClientStore(jujuclienttesting.MinimalStore())

	_, err := cmdtesting.RunCommand(c, modelcmd.WrapBase(unexposeCmd), args...)
	return err
}

func (s *UnexposeSuite) TestUnexpose(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Unexpose(gomock.Any(), "some-application-name", []string(nil)).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runUnexpose(c, api, "some-application-name")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UnexposeSuite) TestUnexposeEndpoints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Unexpose(gomock.Any(), "some-application-name", []string{"ep1", "ep2"}).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runUnexpose(c, api, "some-application-name", "--endpoints", "ep1,ep2")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UnexposeSuite) TestBlockUnexpose(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Unexpose(gomock.Any(), "some-application-name", []string(nil)).Return(apiservererrors.OperationBlockedError("unexpose"))
	api.EXPECT().Close().Return(nil)

	err := runUnexpose(c, api, "some-application-name")
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), tc.IsTrue)
}
