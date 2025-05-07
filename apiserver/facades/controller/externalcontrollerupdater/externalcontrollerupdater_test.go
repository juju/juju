// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater"
	"github.com/juju/juju/core/crossmodel"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	coretesting.BaseSuite

	resources *common.Resources
}

func (s *CrossControllerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })
}

func (s *CrossControllerSuite) TestExternalControllerInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ecService := NewMockECService(ctrl)

	ctrlTag, err := names.ParseControllerTag(coretesting.ControllerTag.String())
	c.Assert(err, jc.ErrorIsNil)
	ecService.EXPECT().Controller(gomock.Any(), ctrlTag.Id()).Return(&crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "foo",
		Addrs:          []string{"bar"},
		CACert:         "baz",
	}, nil)

	modelTag, err := names.ParseControllerTag("controller-" + coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	ecService.EXPECT().Controller(gomock.Any(), modelTag.Id()).Return(nil, errors.NotFoundf("external controller with UUID deadbeef-0bad-400d-8000-4b1d0d06f00d"))

	api, err := externalcontrollerupdater.NewAPI(s.resources, ecService)
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.ExternalControllerInfo(context.Background(), params.Entities{
		Entities: []params.Entity{
			{coretesting.ControllerTag.String()},
			{"controller-" + coretesting.ModelTag.Id()},
			{"machine-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ExternalControllerInfoResults{
		[]params.ExternalControllerInfoResult{{
			Result: &params.ExternalControllerInfo{
				ControllerTag: coretesting.ControllerTag.String(),
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			Error: &params.Error{
				Code:    "not found",
				Message: `external controller with UUID deadbeef-0bad-400d-8000-4b1d0d06f00d not found`,
			},
		}, {
			Error: &params.Error{Message: `"machine-42" is not a valid controller tag`},
		}},
	})
}

func (s *CrossControllerSuite) TestSetExternalControllerInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ecService := NewMockECService(ctrl)

	firstControllerTag := coretesting.ControllerTag.String()
	firstControllerTagParsed, err := names.ParseControllerTag(firstControllerTag)
	c.Assert(err, jc.ErrorIsNil)
	secondControllerTag := "controller-" + coretesting.ModelTag.Id()
	secondControllerTagParsed, err := names.ParseControllerTag(secondControllerTag)
	c.Assert(err, jc.ErrorIsNil)

	ecService.EXPECT().UpdateExternalController(gomock.Any(), crossmodel.ControllerInfo{
		ControllerUUID: firstControllerTagParsed.Id(),
		Alias:          "foo",
		Addrs:          []string{"bar"},
		CACert:         "baz",
	})
	ecService.EXPECT().UpdateExternalController(gomock.Any(), crossmodel.ControllerInfo{
		ControllerUUID: secondControllerTagParsed.Id(),
		Alias:          "qux",
		Addrs:          []string{"quux"},
		CACert:         "quuz",
	})

	api, err := externalcontrollerupdater.NewAPI(s.resources, ecService)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.SetExternalControllerInfo(context.Background(), params.SetExternalControllersInfoParams{
		[]params.SetExternalControllerInfoParams{{
			params.ExternalControllerInfo{
				ControllerTag: firstControllerTag,
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			params.ExternalControllerInfo{
				ControllerTag: secondControllerTag,
				Alias:         "qux",
				Addrs:         []string{"quux"},
				CACert:        "quuz",
			},
		}, {
			params.ExternalControllerInfo{
				ControllerTag: "machine-42",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		[]params.ErrorResult{
			{nil},
			{nil},
			{Error: &params.Error{Message: `"machine-42" is not a valid controller tag`}},
		},
	})
}

func (s *CrossControllerSuite) TestWatchExternalControllers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ecService := NewMockECService(ctrl)
	mockKeysWatcher := NewMockStringsWatcher(ctrl)
	ecService.EXPECT().Watch().Return(mockKeysWatcher, nil)
	changes := make(chan []string, 1)
	mockKeysWatcher.EXPECT().Changes().Return(changes)
	mockKeysWatcher.EXPECT().Kill().AnyTimes()
	mockKeysWatcher.EXPECT().Wait().Return(nil).AnyTimes()

	api, err := externalcontrollerupdater.NewAPI(s.resources, ecService)
	c.Assert(err, jc.ErrorIsNil)

	changes <- []string{"a", "b"} // initial value

	results, err := api.WatchExternalControllers(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{"a", "b"},
		}},
	})
	c.Assert(s.resources.Get("1"), tc.Equals, mockKeysWatcher)
}

func (s *CrossControllerSuite) TestWatchControllerInfoError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ecService := NewMockECService(ctrl)
	mockKeysWatcher := NewMockStringsWatcher(ctrl)
	ecService.EXPECT().Watch().Return(mockKeysWatcher, nil)
	changes := make(chan []string, 1)
	mockKeysWatcher.EXPECT().Changes().Return(changes)
	mockKeysWatcher.EXPECT().Kill().AnyTimes()
	mockKeysWatcher.EXPECT().Wait().Return(errors.New("nope")).AnyTimes()

	close(changes)

	api, err := externalcontrollerupdater.NewAPI(s.resources, ecService)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.WatchExternalControllers(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{{
			Error: &params.Error{Message: "watching external controllers changes: nope"},
		}},
	})
	c.Assert(s.resources.Get("1"), tc.IsNil)
}
