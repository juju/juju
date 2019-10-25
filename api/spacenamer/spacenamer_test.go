// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/spacenamer"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

type spaceNamerSuite struct {
	jujutesting.BaseSuite

	tag names.Tag

	fCaller   *apimocks.MockFacadeCaller
	apiCaller *apimocks.MockAPICallCloser
}

var _ = gc.Suite(&spaceNamerSuite{})

func (s *spaceNamerSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.BaseSuite.SetUpTest(c)
}

func (s *spaceNamerSuite) TestSetDefaultSpaceName(c *gc.C) {
	defer s.setup(c).Finish()

	resultSource := params.ErrorResult{}

	s.fCaller.EXPECT().FacadeCall("SetDefaultSpaceName", nil, gomock.Any()).SetArg(2, resultSource)

	api := spacenamer.NewClientFromFacade(s.fCaller)
	err := api.SetDefaultSpaceName()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceNamerSuite) TestWatchDefaultSpaceConfig(c *gc.C) {
	defer s.setup(c).Finish()

	resultSource := params.NotifyWatchResult{}

	s.fCaller.EXPECT().FacadeCall("WatchDefaultSpaceConfig", nil, gomock.Any()).SetArg(2, resultSource)

	api := spacenamer.NewClientFromFacade(s.fCaller)
	_, err := api.WatchDefaultSpaceConfig()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceNamerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiCaller = apimocks.NewMockAPICallCloser(ctrl)

	s.fCaller = apimocks.NewMockFacadeCaller(ctrl)
	s.fCaller.EXPECT().RawAPICaller().Return(s.apiCaller).AnyTimes()

	return ctrl
}
