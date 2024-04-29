// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/clock"
	"github.com/juju/os/v2/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type SupportedSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SupportedSuite{})

func (s *SupportedSuite) TestCompileForControllers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := clock.WallClock.Now()

	mockDistroSource := NewMockDistroSource(ctrl)
	mockDistroSource.EXPECT().Refresh().Return(nil)
	mockDistroSource.EXPECT().SeriesInfo("supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("deprecated-lts").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("not-updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 1),
		EOL:      now.AddDate(0, 0, 2),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("ignored").Return(series.DistroInfoSerie{}, false)

	preset := map[SeriesName]seriesVersion{
		"supported": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.1",
			Supported:    true,
		},
		"deprecated-lts": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.2",
			Supported:    false,
		},
		"not-updated": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.3",
			Supported:    false,
		},
		"ignored": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.4",
			Supported:    false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()

	c.Assert(ctrlBases, jc.DeepEquals, []Base{MustParseBaseFromString("foo@1.1.1")})
}

func (s *SupportedSuite) TestCompileForControllersWithOverride(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := clock.WallClock.Now()

	mockDistroSource := NewMockDistroSource(ctrl)
	mockDistroSource.EXPECT().Refresh().Return(nil)
	mockDistroSource.EXPECT().SeriesInfo("supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 9),
		EOL:      now.AddDate(0, 0, 10),
	}, true)

	preset := map[SeriesName]seriesVersion{
		"supported": {
			WorkloadType:           ControllerWorkloadType,
			OS:                     "foo",
			Version:                "1.1.1",
			Supported:              true,
			IgnoreDistroInfoUpdate: true,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()

	c.Assert(ctrlBases, jc.DeepEquals, []Base{MustParseBaseFromString("foo@1.1.1")})
}

func (s *SupportedSuite) TestCompileForControllersNoUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := clock.WallClock.Now()

	mockDistroSource := NewMockDistroSource(ctrl)
	mockDistroSource.EXPECT().Refresh().Return(nil)
	mockDistroSource.EXPECT().SeriesInfo("supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 9),
		EOL:      now.AddDate(0, 0, 10),
	}, true)

	preset := map[SeriesName]seriesVersion{
		"supported": {
			WorkloadType:           ControllerWorkloadType,
			OS:                     "foo",
			Version:                "1.1.1",
			Supported:              false,
			IgnoreDistroInfoUpdate: false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()

	c.Assert(ctrlBases, jc.DeepEquals, []Base{})
}

func (s *SupportedSuite) TestCompileForControllersUpdated(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := clock.WallClock.Now()

	mockDistroSource := NewMockDistroSource(ctrl)
	mockDistroSource.EXPECT().Refresh().Return(nil)
	mockDistroSource.EXPECT().SeriesInfo("supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -10),
		EOL:      now.AddDate(0, 0, -9),
	}, true)

	preset := map[SeriesName]seriesVersion{
		"supported": {
			WorkloadType:           ControllerWorkloadType,
			OS:                     "foo",
			Version:                "1.1.1",
			Supported:              true,
			IgnoreDistroInfoUpdate: false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()

	c.Assert(ctrlBases, jc.DeepEquals, []Base{})
}

func (s *SupportedSuite) TestCompileForControllersWithoutOverride(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := clock.WallClock.Now()

	mockDistroSource := NewMockDistroSource(ctrl)
	mockDistroSource.EXPECT().Refresh().Return(nil)
	mockDistroSource.EXPECT().SeriesInfo("supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 9),
		EOL:      now.AddDate(0, 0, 10),
	}, true)

	preset := map[SeriesName]seriesVersion{
		"supported": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.1",
			Supported:    true,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlBases := info.controllerBases()

	c.Assert(ctrlBases, jc.DeepEquals, []Base{})
}

func (s *SupportedSuite) TestCompileForWorkloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	now := clock.WallClock.Now()

	mockDistroSource := NewMockDistroSource(ctrl)
	mockDistroSource.EXPECT().Refresh().Return(nil)
	mockDistroSource.EXPECT().SeriesInfo("ctrl-supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("ctrl-deprecated-lts").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("ctrl-not-updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 1),
		EOL:      now.AddDate(0, 0, 2),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("ctrl-ignored").Return(series.DistroInfoSerie{}, false)
	mockDistroSource.EXPECT().SeriesInfo("work-supported").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("work-deprecated-lts").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("work-not-updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 1),
		EOL:      now.AddDate(0, 0, 2),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("work-ignored").Return(series.DistroInfoSerie{}, false)

	preset := map[SeriesName]seriesVersion{
		"ctrl-supported": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.1",
			Supported:    true,
		},
		"ctrl-deprecated-lts": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.2",
			Supported:    false,
		},
		"ctrl-not-updated": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.3",
			Supported:    false,
		},
		"ctrl-ignored": {
			WorkloadType: ControllerWorkloadType,
			OS:           "foo",
			Version:      "1.1.4",
			Supported:    false,
		},
		"work-supported": {
			WorkloadType: OtherWorkloadType,
			OS:           "foo",
			Version:      "1.1.5",
			Supported:    true,
		},
		"work-deprecated-lts": {
			WorkloadType: OtherWorkloadType,
			OS:           "foo",
			Version:      "1.1.6",
			Supported:    false,
		},
		"work-not-updated": {
			WorkloadType: OtherWorkloadType,
			OS:           "foo",
			Version:      "1.1.7",
			Supported:    false,
		},
		"work-ignored": {
			WorkloadType: OtherWorkloadType,
			OS:           "foo",
			Version:      "1.1.8",
			Supported:    false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	workloadBases := info.workloadBases(false)

	c.Assert(workloadBases, jc.DeepEquals, []Base{MustParseBaseFromString("foo@1.1.1"), MustParseBaseFromString("foo@1.1.5")})

	// Double check that controller series doesn't change when we have workload
	// types.
	ctrlBases := info.controllerBases()

	c.Assert(ctrlBases, jc.DeepEquals, []Base{MustParseBaseFromString("foo@1.1.1")})
}
