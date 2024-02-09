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
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type ExposeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ExposeSuite{})

func runExpose(c *gc.C, api ApplicationExposeAPI, args ...string) error {
	exposeCmd := &exposeCommand{api: api}
	exposeCmd.SetClientStore(jujuclienttesting.MinimalStore())

	_, err := cmdtesting.RunCommand(c, modelcmd.WrapBase(exposeCmd), args...)
	return err
}

func (s *ExposeSuite) TestExpose(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Expose("some-application-name", nil).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runExpose(c, api, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExposeSuite) TestExposeEndpoints(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Expose("some-application-name", map[string]params.ExposedEndpoint{
		"ep1": {}, "ep2": {},
	}).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runExpose(c, api, "some-application-name", "--endpoints", "ep1,ep2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExposeSuite) TestExposeSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Expose("some-application-name", map[string]params.ExposedEndpoint{
		"ep1": {ExposeToSpaces: []string{"sp1", "sp2"}},
	}).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runExpose(c, api, "some-application-name", "--endpoints", "ep1", "--to-spaces", "sp1,sp2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExposeSuite) TestExposeCIDRS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Expose("some-application-name", map[string]params.ExposedEndpoint{
		"ep1": {ExposeToCIDRs: []string{"cidr1", "cidr2"}},
	}).Return(nil)
	api.EXPECT().Close().Return(nil)

	err := runExpose(c, api, "some-application-name", "--endpoints", "ep1", "--to-cidrs", "cidr1,cidr2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExposeSuite) TestBlockExpose(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := mocks.NewMockApplicationExposeAPI(ctrl)
	api.EXPECT().Expose("some-application-name", nil).Return(apiservererrors.OperationBlockedError("expose"))
	api.EXPECT().Close().Return(nil)

	err := runExpose(c, api, "some-application-name")
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), jc.IsTrue)
}
