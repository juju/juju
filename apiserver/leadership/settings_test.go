package leadership

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type settingsSuite struct{}

func (s *settingsSuite) TestReadSettings(c *gc.C) {

	settingsToReturn := map[string]interface{}{"foo": "bar"}
	numGetSettingCalls := 0
	getSettings := func(serviceId string) (map[string]interface{}, error) {
		numGetSettingCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		return settingsToReturn, nil
	}
	stubAuthorizer := &stubAuthorizer{}
	accessor := NewLeadershipSettingsAccessor(stubAuthorizer, nil, getSettings, nil, nil)

	results, err := accessor.Read(params.GetLeadershipSettingsBulkParams{
		[]params.GetLeadershipSettingsParams{
			{ServiceTag: names.NewServiceTag(StubServiceNm).String()},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(numGetSettingCalls, gc.Equals, 1)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	// NOTE: Beware of map-ordering if you add more keys-values to
	// "settingsToReturn".
	c.Check(results.Results[0].Settings, gc.DeepEquals, settingsToReturn)
}

func (s *settingsSuite) TestWriteSettings(c *gc.C) {
	settingsToReturn := map[string]interface{}{"foo": "bar"}

	numGetSettingCalls := 0
	getSettings := func(serviceId string) (map[string]interface{}, error) {
		numGetSettingCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		return settingsToReturn, nil
	}

	numWriteSettingCalls := 0
	writeSettings := func(serviceId string, settings map[string]interface{}) error {
		numWriteSettingCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		return nil
	}

	numIsLeaderCalls := 0
	isLeader := func(serviceId, unitId string) bool {
		numIsLeaderCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		c.Check(unitId, gc.Equals, StubUnitNm)
		return true
	}

	accessor := NewLeadershipSettingsAccessor(&stubAuthorizer{}, nil, getSettings, writeSettings, isLeader)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		[]params.MergeLeadershipSettingsParam{
			{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				Settings:   map[string]interface{}{"baz": "biz"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
}

func (s *settingsSuite) TestBlockUntilChanges(c *gc.C) {

	numSettingsWatcherCalls := 0
	settingsNotifier := func(serviceId string) <-chan struct{} {
		numSettingsWatcherCalls++
		c.Check(serviceId, gc.Equals, StubServiceNm)
		notifier := make(chan struct{})
		go func() { notifier <- struct{}{} }()
		return notifier
	}

	accessor := NewLeadershipSettingsAccessor(&stubAuthorizer{}, settingsNotifier, nil, nil, nil)

	results, err := accessor.BlockUntilChanges(params.LeadershipWatchSettingsParam{
		names.NewServiceTag(StubServiceNm).String(),
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}
