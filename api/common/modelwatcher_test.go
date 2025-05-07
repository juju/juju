// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"

	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type modelwatcherTests struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&modelwatcherTests{})

func (s *modelwatcherTests) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *modelwatcherTests) TestModelConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	attrs := testing.FakeConfig()
	attrs["logging-config"] = "<root>=INFO"

	facade := apimocks.NewMockFacadeCaller(ctrl)
	result := params.ModelConfigResult{
		Config: params.ModelConfig(attrs),
	}
	facade.EXPECT().FacadeCall(gomock.Any(), "ModelConfig", nil, gomock.Any()).SetArg(3, result).Return(nil)

	client := common.NewModelConfigWatcher(facade)
	cfg, err := client.ModelConfig(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(testing.Attrs(cfg.AllAttrs()), tc.DeepEquals, attrs)
}

func (s *modelwatcherTests) TestWatchForModelConfigChanges(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	facade := apimocks.NewMockFacadeCaller(ctrl)
	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("NotifyWatcher").Return(666)
	caller.EXPECT().APICall(gomock.Any(), "NotifyWatcher", 666, "", "Next", nil, gomock.Any()).Return(nil).AnyTimes()
	caller.EXPECT().APICall(gomock.Any(), "NotifyWatcher", 666, "", "Stop", nil, gomock.Any()).Return(nil).AnyTimes()

	result := params.NotifyWatchResult{}
	facade.EXPECT().FacadeCall(gomock.Any(), "WatchForModelConfigChanges", nil, gomock.Any()).SetArg(3, result).Return(nil)
	facade.EXPECT().RawAPICaller().Return(caller)

	client := common.NewModelConfigWatcher(facade)
	w, err := client.WatchForModelConfigChanges(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-w.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}
