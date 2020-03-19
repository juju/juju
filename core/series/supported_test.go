// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	series "github.com/juju/os/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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
	mockDistroSource.EXPECT().SeriesInfo("updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("not-updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 1),
		EOL:      now.AddDate(0, 0, 2),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("ignored").Return(series.DistroInfoSerie{}, false)

	preset := map[SeriesName]SeriesVersion{
		"supported": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    true,
		},
		"updated": {
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

	info := NewSupportedInfo(mockDistroSource, preset)
	err := info.Compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{"supported", "updated"})
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

	preset := map[SeriesName]SeriesVersion{
		"supported": {
			WorkloadType:           ControllerWorkloadType,
			Version:                "1.1.1",
			Supported:              true,
			IgnoreDistroInfoUpdate: true,
		},
	}

	info := NewSupportedInfo(mockDistroSource, preset)
	err := info.Compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{"supported"})
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

	preset := map[SeriesName]SeriesVersion{
		"supported": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    true,
		},
	}

	info := NewSupportedInfo(mockDistroSource, preset)
	err := info.Compile(now)
	c.Assert(err, jc.ErrorIsNil)

	ctrlSeries := info.ControllerSeries()
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
	mockDistroSource.EXPECT().SeriesInfo("ctrl-updated").Return(series.DistroInfoSerie{
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
	mockDistroSource.EXPECT().SeriesInfo("work-updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, -1),
		EOL:      now.AddDate(0, 0, 1),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("work-not-updated").Return(series.DistroInfoSerie{
		Released: now.AddDate(0, 0, 1),
		EOL:      now.AddDate(0, 0, 2),
	}, true)
	mockDistroSource.EXPECT().SeriesInfo("work-ignored").Return(series.DistroInfoSerie{}, false)

	preset := map[SeriesName]SeriesVersion{
		"ctrl-supported": {
			WorkloadType: ControllerWorkloadType,
			Version:      "1.1.1",
			Supported:    true,
		},
		"ctrl-updated": {
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
		"work-updated": {
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

	info := NewSupportedInfo(mockDistroSource, preset)
	err := info.Compile(now)
	c.Assert(err, jc.ErrorIsNil)

	workSeries := info.WorkloadSeries()
	sort.Strings(workSeries)

	c.Assert(workSeries, jc.DeepEquals, []string{"ctrl-supported", "ctrl-updated", "work-supported", "work-updated"})

	// Double check that controller series doesn't change when we have workload
	// types.
	ctrlSeries := info.ControllerSeries()
	sort.Strings(ctrlSeries)

	c.Assert(ctrlSeries, jc.DeepEquals, []string{"ctrl-supported", "ctrl-updated"})
}
