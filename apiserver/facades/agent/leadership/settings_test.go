// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/leadership"
	"github.com/juju/juju/apiserver/params"
	coreleadership "github.com/juju/juju/core/leadership"
)

// TODO(fwereade): this is *severely* undertested.
type settingsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&settingsSuite{})

func (s *settingsSuite) TestReadSettings(c *gc.C) {

	settingsToReturn := params.Settings(map[string]string{"foo": "bar"})
	numGetSettingCalls := 0
	getSettings := func(serviceId string) (map[string]string, error) {
		numGetSettingCalls++
		c.Check(serviceId, gc.Equals, StubAppNm)
		return settingsToReturn, nil
	}
	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, getSettings, nil, nil)

	results, err := accessor.Read(params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(StubAppNm).String()},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(numGetSettingCalls, gc.Equals, 1)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Check(results.Results[0].Settings, gc.DeepEquals, settingsToReturn)
}

func (s *settingsSuite) TestWriteSettings(c *gc.C) {

	expectToken := &fakeToken{}

	numLeaderCheckCalls := 0
	leaderCheck := func(appName, unitId string) coreleadership.Token {
		numLeaderCheckCalls++
		c.Check(appName, gc.Equals, StubAppNm)
		c.Check(unitId, gc.Equals, StubUnitNm)
		return expectToken
	}

	numWriteSettingCalls := 0
	writeSettings := func(token coreleadership.Token, serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		c.Check(serviceId, gc.Equals, StubAppNm)
		c.Check(token, gc.Equals, expectToken)
		c.Check(settings, jc.DeepEquals, map[string]string{"baz": "biz"})
		return nil
	}

	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, nil, leaderCheck, writeSettings)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: names.NewApplicationTag(StubAppNm).String(),
				Settings:       map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
	c.Check(numWriteSettingCalls, gc.Equals, 1)
	c.Check(numLeaderCheckCalls, gc.Equals, 1)
}

func (s *settingsSuite) TestWriteSettingsWrongUnit(c *gc.C) {

	numLeaderCheckCalls := 0
	leaderCheck := func(appName, unitId string) coreleadership.Token {
		numLeaderCheckCalls++
		return &fakeToken{}
	}

	numWriteSettingCalls := 0
	writeSettings := func(token coreleadership.Token, serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		return nil
	}

	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, nil, leaderCheck, writeSettings)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: names.NewApplicationTag(StubAppNm).String(),
				UnitTag:        names.NewUnitTag("foo/0").String(),
				Settings:       map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Check(numWriteSettingCalls, gc.Equals, 0)
	c.Check(numLeaderCheckCalls, gc.Equals, 0)
}

func (s *settingsSuite) TestWriteSettingsError(c *gc.C) {

	expectToken := &fakeToken{}

	numLeaderCheckCalls := 0
	leaderCheck := func(serviceId, unitId string) coreleadership.Token {
		numLeaderCheckCalls++
		c.Check(serviceId, gc.Equals, StubAppNm)
		c.Check(unitId, gc.Equals, StubUnitNm)
		return expectToken
	}

	numWriteSettingCalls := 0
	writeSettings := func(token coreleadership.Token, serviceId string, settings map[string]string) error {
		numWriteSettingCalls++
		c.Check(serviceId, gc.Equals, StubAppNm)
		c.Check(token, gc.Equals, expectToken)
		c.Check(settings, jc.DeepEquals, map[string]string{"baz": "biz"})
		return errors.New("zap blort")
	}

	authorizer := stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, nil, nil, leaderCheck, writeSettings)

	results, err := accessor.Merge(params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: names.NewApplicationTag(StubAppNm).String(),
				Settings:       map[string]string{"baz": "biz"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "zap blort")
	c.Check(numWriteSettingCalls, gc.Equals, 1)
	c.Check(numLeaderCheckCalls, gc.Equals, 1)
}

func (s *settingsSuite) TestBlockUntilChanges(c *gc.C) {

	numSettingsWatcherCalls := 0
	registerWatcher := func(appName string) (string, error) {
		numSettingsWatcherCalls++
		c.Check(appName, gc.Equals, StubAppNm)
		return "foo", nil
	}

	authorizer := &stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	accessor := leadership.NewLeadershipSettingsAccessor(authorizer, registerWatcher, nil, nil, nil)

	results, err := accessor.WatchLeadershipSettings(params.Entities{Entities: []params.Entity{
		{Tag: names.NewApplicationTag(StubAppNm).String()},
	}})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

type fakeToken struct {
	coreleadership.Token
}
