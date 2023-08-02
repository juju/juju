// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"sort"

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
			Version:      "1.1.1",
			Supported:    true,
		},
		"deprecated-lts": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"not-updated": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"ignored": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.controllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{"supported"})
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
			Version:                "1.1.1",
			Supported:              true,
			IgnoreDistroInfoUpdate: true,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.controllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{"supported"})
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
			Version:                "1.1.1",
			Supported:              false,
			IgnoreDistroInfoUpdate: false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.controllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{})
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
			Version:                "1.1.1",
			Supported:              true,
			IgnoreDistroInfoUpdate: false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.controllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{})
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
			Version:      "1.1.1",
			Supported:    true,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.controllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{})
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
			Version:      "1.1.1",
			Supported:    true,
		},
		"ctrl-deprecated-lts": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"ctrl-not-updated": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"ctrl-ignored": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"work-supported": {
			WorkloadType: OtherWorkloadType,
			Version:      "1.1.1",
			Supported:    true,
		},
		"work-deprecated-lts": {
			WorkloadType: OtherWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"work-not-updated": {
			WorkloadType: OtherWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
		"work-ignored": {
			WorkloadType: OtherWorkloadType,
			Version:      "1.1.1",
			Supported:    false,
		},
	}

	info := newSupportedInfo(mockDistroSource, preset)
	err := info.compile(now)
	c.Assert(err, jc.ErrorIsNil)

	workSeries := info.workloadSeries(false)
	sort.Strings(workSeries)

	c.Assert(workSeries, jc.DeepEquals, []string{"ctrl-supported", "work-supported"})

	// Double check that controller series doesn't change when we have workload
	// types.
	ctrlSeries := info.controllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{"ctrl-supported"})
}
