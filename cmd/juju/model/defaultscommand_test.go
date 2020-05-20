// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type DefaultsCommandSuite struct {
	fakeModelDefaultEnvSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&DefaultsCommandSuite{})

func (s *DefaultsCommandSuite) SetUpTest(c *gc.C) {
	s.fakeModelDefaultEnvSuite.SetUpTest(c)
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "controller"
	s.store.Controllers["controller"] = jujuclient.ControllerDetails{}
	s.store.Models["controller"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"king/fred": {ModelUUID: testing.ModelTag.Id(), ModelType: "iaas"},
		},
		CurrentModel: "king/fred",
	}
	s.store.Accounts["controller"] = jujuclient.AccountDetails{
		User: "king",
	}
}

func (s *DefaultsCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewDefaultsCommandForTest(s.fakeAPIRoot, s.fakeDefaultsAPI, s.fakeCloudAPI, s.store)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *DefaultsCommandSuite) TestDefaultsInit(c *gc.C) {
	for i, test := range []struct {
		description string
		args        []string
		errorMatch  string
		nilErr      bool
	}{{
		// Test set
		description: "test cannot set agent-version",
		args:        []string{"agent-version=2.0.0"},
		errorMatch:  `"agent-version" must be set via "upgrade-model"`,
	}, {
		description: "test set multiple keys",
		args:        []string{"foo=bar", "baz=eggs"},
		nilErr:      true,
	}, {
		// Test reset
		description: "test empty args with reset fails",
		args:        []string{"--reset"},
		errorMatch:  "option needs an argument: --reset",
	}, {
		description: "test reset with positional arg interpereted as invalid region",
		args:        []string{"--reset", "something", "weird"},
		errorMatch:  `invalid region specified: "weird"`,
	}, {
		description: "test reset with valid region and duplicate key set",
		args:        []string{"--reset", "something", "dummy-region", "something=weird"},
		errorMatch:  `key "something" cannot be both set and unset in the same command`,
	}, {
		description: "test reset with valid region and extra positional arg",
		args:        []string{"--reset", "something", "dummy-region", "weird"},
		errorMatch:  "cannot retrieve defaults for a region and reset attributes at the same time",
	}, {
		description: "test reset with valid region only",
		args:        []string{"--reset", "foo", "dummy-region"},
		nilErr:      true,
	}, {
		description: "test cannot reset agent version",
		args:        []string{"--reset", "agent-version"},
		errorMatch:  `"agent-version" cannot be reset`,
	}, {
		description: "test reset inits",
		args:        []string{"--reset", "foo"},
		nilErr:      true,
	}, {
		description: "test trailing reset fails",
		args:        []string{"foo=bar", "--reset"},
		errorMatch:  "option needs an argument: --reset",
	}, {
		description: "test reset and get init",
		args:        []string{"--reset", "agent-version,b", "foo=bar"},
		errorMatch:  `"agent-version" cannot be reset`,
	}, {
		description: "test reset with key=val fails",
		args:        []string{"--reset", "foo=bar"},
		errorMatch:  `--reset accepts a comma delimited set of keys "a,b,c", received: "foo=bar"`,
	}, {
		description: "test reset multiple with key=val fails",
		args:        []string{"--reset", "a,foo=bar,b"},
		errorMatch:  `--reset accepts a comma delimited set of keys "a,b,c", received: "foo=bar"`,
	}, {
		description: "test reset with two positional args fails expecting a region",
		args:        []string{"--reset", "a", "b", "c"},
		errorMatch:  `invalid region specified: "b"`,
	}, {
		description: "test reset with two positional args fails expecting a region reordered",
		args:        []string{"a", "--reset", "b", "c"},
		errorMatch:  `invalid region specified: "a"`,
	}, {
		description: "test multiple reset inits",
		args:        []string{"--reset", "a", "--reset", "b"},
		nilErr:      true,
	}, {
		description: "test multiple reset and set inits",
		args:        []string{"--reset", "a", "b=c", "--reset", "d"},
		nilErr:      true,
	}, {
		description: "test multiple reset with valid region inits",
		args:        []string{"dummy-region", "--reset", "a", "--reset", "b"},
		nilErr:      true,
	}, {
		description: "test multiple reset with two positional args fails expecting a region reordered",
		args:        []string{"a", "--reset", "b", "--reset", "c", "d"},
		errorMatch:  `invalid region specified: "a"`,
	}, {
		description: "test reset multiple with key=val fails",
		args:        []string{"--reset", "a", "--reset", "b,foo=bar,c"},
		errorMatch:  `--reset accepts a comma delimited set of keys "a,b,c", received: "foo=bar"`,
	}, {
		// test get
		description: "test no args inits",
		args:        nil,
		nilErr:      true,
	}, {
		description: "one key arg inits",
		args:        []string{"attr"},
		nilErr:      true,
	}, {
		description: "test two key args fails",
		args:        []string{"one", "two"},
		errorMatch:  "can only retrieve defaults for one key or all",
	}, {
		description: "test multiple key args fails",
		args:        []string{"one", "two", "three"},
		errorMatch:  "can only retrieve defaults for one key or all",
	}, {
		description: "test valid region and one arg",
		args:        []string{"dummy-region", "attr2"},
		nilErr:      true,
	}, {
		description: "test valid cloud and no args",
		args:        []string{"dummy"},
		nilErr:      true,
	}, {
		description: "test valid region and no args",
		args:        []string{"dummy-region"},
		nilErr:      true,
	}, {
		// test cloud/region
		description: "test invalid cloud fails",
		args:        []string{"invalidCloud/invalidRegion", "one=two"},
		errorMatch:  `cloud "invalidCloud" not found`,
	}, {
		description: "test valid cloud with invalid region fails",
		args:        []string{"dummy/invalidRegion", "one=two"},
		errorMatch:  `invalid cloud or region specified: "dummy/invalidRegion"`,
	}, {
		description: "test no cloud with invalid region fails",
		args:        []string{"invalidRegion", "one=two"},
		errorMatch:  `invalid cloud or region specified: "invalidRegion"`,
	}, {
		description: "test valid region with set arg succeeds",
		args:        []string{"dummy-region", "one=two"},
		nilErr:      true,
	}, {
		description: "test valid cloud with set and reset succeeds",
		args:        []string{"dummy", "one=two", "--reset", "three"},
		nilErr:      true,
	}, {
		description: "test valid region with set and reset succeeds",
		args:        []string{"dummy-region", "one=two", "--reset", "three"},
		nilErr:      true,
	}, {
		description: "test reset and set with extra key is interpereted as invalid region",
		args:        []string{"--reset", "something,else", "invalidRegion", "is=weird"},
		errorMatch:  `invalid cloud or region specified: "invalidRegion"`,
	}, {
		description: "test reset and set with valid region and extra key fails",
		args:        []string{"--reset", "something,else", "dummy-region", "invalidkey", "is=weird"},
		errorMatch:  "cannot set and retrieve default values simultaneously",
	}, {
		// test various invalid
		description: "test too many positional args with reset",
		args:        []string{"--reset", "a", "b", "c", "d"},
		errorMatch:  "invalid input",
	}, {
		description: "test too many positional args with invalid region set",
		args:        []string{"a", "a=b", "b", "c=d"},
		errorMatch:  `invalid cloud or region specified: "a"`,
	}, {
		description: "test invalid positional args with set",
		args:        []string{"a=b", "b", "c=d"},
		errorMatch:  `.*(no such file or directory|cannot find the file specified).*`,
	}, {
		description: "test invalid positional args with set and trailing key",
		args:        []string{"a=b", "c=d", "e"},
		errorMatch:  "cannot set and retrieve default values simultaneously",
	}, {
		description: "test invalid positional args with valid region, set, reset",
		args:        []string{"dummy-region", "a=b", "--reset", "c,d,", "e=f", "g"},
		errorMatch:  "cannot set and retrieve default values simultaneously",
	}, {
		// Test some random orderings
		description: "test invalid positional args with set, reset with trailing comman and split key=values",
		args:        []string{"dummy-region", "a=b", "--reset", "c,d,", "e=f"},
		nilErr:      true,
	}, {
		description: "test leading comma with reset",
		args:        []string{"--reset", ",a,b"},
		nilErr:      true,
	}} {
		c.Logf("test %d: %s", i, test.description)
		_, err := s.run(c, test.args...)
		if test.nilErr {
			c.Check(err, jc.ErrorIsNil)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.errorMatch)
	}
}

func (s *DefaultsCommandSuite) TestMultiCloudMessage(c *gc.C) {
	s.fakeCloudAPI.clouds[names.NewCloudTag("another")] = cloud.Cloud{Name: "another"}
	_, err := s.run(c, "attr")
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "You haven't specified a cloud and more than one exists on this controller.*another,dummy")
}

func (s *DefaultsCommandSuite) TestNoVisibleCloudMessage(c *gc.C) {
	s.fakeCloudAPI.clouds = nil
	_, err := s.run(c, "attr")
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "You don't have access to any clouds on this controller.Only controller administrators can set default model values.")
}

func (s *DefaultsCommandSuite) TestResetUnknownValueLogs(c *gc.C) {
	ctx, err := s.run(c, "--reset", "attr,weird")
	c.Assert(err, jc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
	c.Check(cmdtesting.Stdout(ctx), jc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestResetAttr(c *gc.C) {
	ctx, err := s.run(c, "--reset", "attr,unknown")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, gc.Equals, "dummy")
	c.Check(s.fakeDefaultsAPI.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
	})
	c.Check(cmdtesting.Stdout(ctx), jc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestResetRegionAttr(c *gc.C) {
	ctx, err := s.run(c, "--reset", "attr,unknown", "dummy-region")
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.fakeDefaultsAPI.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
	})
	c.Check(cmdtesting.Stdout(ctx), jc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestResetBlockedError(c *gc.C) {
	s.fakeDefaultsAPI.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "attr")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *DefaultsCommandSuite) TestSetUnknownValueLogs(c *gc.C) {
	_, err := s.run(c, "weird=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *DefaultsCommandSuite) TestSet(c *gc.C) {
	_, err := s.run(c, "special=extra", "attr=baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, gc.Equals, "dummy")
	c.Assert(s.fakeDefaultsAPI.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: "baz", Default: nil, Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"special": {Controller: "extra", Default: nil, Regions: nil},
	})
}

func (s *DefaultsCommandSuite) TestSetValueWithSlash(c *gc.C) {
	// A value with a "/" might be interpreted as a cloud/region.
	_, err := s.run(c, `juju-no-proxy="localhost,127.0.0.1,127.0.0.53,10.0.8.0/24"`)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, gc.Equals, "dummy")
	c.Assert(s.fakeDefaultsAPI.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: nil, Default: "foo", Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"juju-no-proxy": {Controller: "localhost,127.0.0.1,127.0.0.53,10.0.8.0/24", Default: nil, Regions: nil},
	})
}

func (s *DefaultsCommandSuite) TestSetFromFile(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("special: extra\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, configFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: nil, Default: "foo", Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"special": {Controller: "extra", Default: nil, Regions: nil},
	})
}

func (s *DefaultsCommandSuite) TestSetFromFileCombined(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("special: extra\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, configFile, "attr=baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: "baz", Default: nil, Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"special": {Controller: "extra", Default: nil, Regions: nil},
	})
}

func (s *DefaultsCommandSuite) TestSetConveysCloudRegion(c *gc.C) {
	table := []struct {
		input, cloud, region string
	}{
		{"", "dummy", ""},
		{"dummy-region", "dummy", "dummy-region"},
		{"dummy/dummy-region", "dummy", "dummy-region"},
		{"another-region", "dummy", "another-region"},
	}
	for i, test := range table {
		c.Logf("test %d", i)
		var err error
		if test.input == "" {
			_, err = s.run(c, "special=extra", "--reset", "attr")
		} else {
			_, err = s.run(c, test.input, "special=extra", "--reset", "attr")
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(s.fakeDefaultsAPI.region, jc.DeepEquals, test.region)
		c.Assert(s.fakeDefaultsAPI.cloud, jc.DeepEquals, test.cloud)
	}
}

func (s *DefaultsCommandSuite) TestBlockedErrorOnSet(c *gc.C) {
	s.fakeDefaultsAPI.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *DefaultsCommandSuite) TestGetSingleValue(c *gc.C) {
	context, err := s.run(c, "attr2")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeDefaultsAPI.cloud, gc.Equals, "dummy")
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"Attribute         Default        Controller\n" +
		"attr2             -              bar\n" +
		"  dummy-region    dummy-value    -\n" +
		"  another-region  another-value  -"
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "attr2")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, gc.Equals,
		`{"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"},{"name":"another-region","value":"another-value"}]}}`)
}

func (s *DefaultsCommandSuite) TestGetAllValuesYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"attr:\n" +
		"  default: foo\n" +
		"attr2:\n" +
		"  controller: bar\n" +
		"  regions:\n" +
		"  - name: dummy-region\n" +
		"    value: dummy-value\n" +
		"  - name: another-region\n" +
		"    value: another-value"
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := `{"attr":{"default":"foo"},"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"},{"name":"another-region","value":"another-value"}]}}`
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetAllValuesTabular(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"Attribute         Default        Controller\n" +
		"attr              foo            -\n" +
		"attr2             -              bar\n" +
		"  dummy-region    dummy-value    -\n" +
		"  another-region  another-value  -"
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetRegionValuesTabular(c *gc.C) {
	context, err := s.run(c, "dummy-region")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"Attribute       Default      Controller\n" +
		"attr2           -            bar\n" +
		"  dummy-region  dummy-value  -"
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetRegionNoValuesTabular(c *gc.C) {
	_, err := s.run(c, "--reset", "attr2")
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := s.run(c, "dummy-region")
	c.Assert(err, gc.ErrorMatches, `there are no default model values in region "dummy-region"`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *DefaultsCommandSuite) TestGetRegionOneArgNoValuesTabular(c *gc.C) {
	ctx, err := s.run(c, "dummy-region", "attr")
	c.Assert(err, gc.ErrorMatches, `there are no default model values for "attr" in region "dummy-region"`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}
