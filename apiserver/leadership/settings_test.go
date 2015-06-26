package leadership

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

func init() {
	gc.Suite(&settingsSuite{})
}

type settingsSuite struct{}

func (s *settingsSuite) TestReadSettings(c *gc.C) {

	settingsToReturn := params.Settings(map[string]string{"foo": "bar"})
	numGetSettingCalls := 0
	getSettings := func(serviceId string) (map[string]string, error) {
		numGetSettingCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		return settingsToReturn, nil
	}
	stubAuthorizer := &stubAuthorizer{}
	accessor := NewLeadershipSettingsAccessor(stubAuthorizer, nil, getSettings, nil, nil)

	results, err := accessor.Read(params.Entities{
		[]params.Entity{
			{Tag: names.NewServiceTag(StubServiceNm).String()},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(numGetSettingCalls, gc.Equals, 1)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Check(results.Results[0].Settings, gc.DeepEquals, settingsToReturn)
}

func (s *settingsSuite) TestWriteSettings(c *gc.C) {

	numWriteSettingCalls := 0
	writeSettings := func(serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		return nil
	}

	numIsLeaderCalls := 0
	isLeader := func(serviceId, unitId string) (bool, error) {
		numIsLeaderCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		c.Check(unitId, gc.Equals, StubUnitNm)
		return true, nil
	}

	accessor := NewLeadershipSettingsAccessor(&stubAuthorizer{}, nil, nil, writeSettings, isLeader)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		[]params.MergeLeadershipSettingsParam{
			{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				Settings:   map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
	c.Check(numWriteSettingCalls, gc.Equals, 1)
	c.Check(numIsLeaderCalls, gc.Equals, 1)
}

func (s *settingsSuite) TestWriteSettingFailsForNonLeader(c *gc.C) {
	numIsLeaderCalls := 0
	isLeader := func(serviceId, unitId string) (bool, error) {
		numIsLeaderCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		c.Check(unitId, gc.Equals, StubUnitNm)
		return false, nil
	}

	accessor := NewLeadershipSettingsAccessor(&stubAuthorizer{}, nil, nil, nil, isLeader)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		[]params.MergeLeadershipSettingsParam{
			{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				Settings:   map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "permission denied")
}

func (s *settingsSuite) TestBlockUntilChanges(c *gc.C) {

	numSettingsWatcherCalls := 0
	registerWatcher := func(serviceId string) (string, error) {
		numSettingsWatcherCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		return "foo", nil
	}

	accessor := NewLeadershipSettingsAccessor(&stubAuthorizer{}, registerWatcher, nil, nil, nil)

	results, err := accessor.WatchLeadershipSettings(params.Entities{[]params.Entity{
		{names.NewServiceTag(StubServiceNm).String()},
	}})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}
