// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	"github.com/juju/charm/v11"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/payloads"
	corepayloads "github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

type ClientSuite struct {
	testing.IsolationSuite

	facade    *mocks.MockFacadeCaller
	apiCaller *mocks.MockAPICallCloser
	client    *payloads.Client
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiCaller = mocks.NewMockAPICallCloser(ctrl)
	s.apiCaller.EXPECT().BestFacadeVersion(gomock.Any()).Return(3).AnyTimes()

	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.facade.EXPECT().RawAPICaller().Return(s.apiCaller).AnyTimes()
	s.facade.EXPECT().BestAPIVersion().Return(1).AnyTimes()
	s.client = payloads.NewClientForTest(s.facade)
	return ctrl
}

func (s *ClientSuite) TestList(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	args := &params.PayloadListArgs{
		Patterns: []string{"a-tag", "a-application/0"},
	}

	payloadResult := params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  corepayloads.StateRunning,
		Labels:  []string{"label"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	}
	resultParams := params.PayloadListResults{
		Results: []params.Payload{payloadResult},
	}
	s.facade.EXPECT().FacadeCall("List", args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	results, err := s.client.ListFull("a-tag", "a-application/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []corepayloads.FullPayloadInfo{
		{
			Payload: corepayloads.Payload{
				PayloadClass: charm.PayloadClass{Name: "spam", Type: "docker"},
				ID:           "idspam",
				Status:       corepayloads.StateRunning,
				Labels:       []string{"label"},
				Unit:         "a-application/0",
			},
			Machine: "1",
		},
	})
}
