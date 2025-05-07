// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type DefaultsCommandSuite struct {
	fakeModelDefaultEnvSuite
	store *jujuclient.MemStore
}

var _ = tc.Suite(&DefaultsCommandSuite{})

func (s *DefaultsCommandSuite) SetUpTest(c *tc.C) {
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

func (s *DefaultsCommandSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	command := model.NewDefaultsCommandForTest(s.fakeAPIRoot, s.fakeDefaultsAPI, s.fakeCloudAPI, s.store)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *DefaultsCommandSuite) TestDefaultsInit(c *tc.C) {
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
		description: "test reset with invalid region",
		args:        []string{"--reset", "something", "--region", "weird"},
		errorMatch:  `invalid region specified: "weird"`,
	}, {
		description: "test valid region, set and reset same key",
		args:        []string{"--reset", "something", "--region", "dummy-region", "something=weird"},
		errorMatch:  `cannot set and reset key "something" simultaneously`,
	}, {
		description: "test reset with valid region and extra positional arg",
		args:        []string{"--reset", "something", "--region", "dummy-region", "weird"},
		errorMatch:  "cannot use --reset flag and get value simultaneously",
	}, {
		description: "test reset with valid region only",
		args:        []string{"--reset", "foo", "--region", "dummy-region"},
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
		errorMatch:  "cannot use --reset flag and get value simultaneously",
	}, {
		description: "test reset with two positional args fails expecting a region reordered",
		args:        []string{"a", "--reset", "b", "c"},
		errorMatch:  "cannot use --reset flag and get value simultaneously",
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
		args:        []string{"--region", "dummy-region", "--reset", "a", "--reset", "b"},
		nilErr:      true,
	}, {
		description: "test multiple reset with two positional args fails expecting a region reordered",
		args:        []string{"a", "--reset", "b", "--reset", "c", "d"},
		errorMatch:  "cannot use --reset flag and get value simultaneously",
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
		errorMatch:  "cannot specify multiple keys to get",
	}, {
		description: "test multiple key args fails",
		args:        []string{"one", "two", "three"},
		errorMatch:  "cannot specify multiple keys to get",
	}, {
		description: "test valid region and one arg",
		args:        []string{"--region", "dummy-region", "attr2"},
		nilErr:      true,
	}, {
		description: "test valid cloud and no args",
		args:        []string{"--cloud", "dummy"},
		nilErr:      true,
	}, {
		description: "test valid region and no args",
		args:        []string{"--region", "dummy-region"},
		nilErr:      true,
	}, {
		// test cloud/region
		description: "test invalid cloud fails",
		args:        []string{"--region", "invalidCloud/invalidRegion", "one=two"},
		errorMatch:  `cloud "invalidCloud" not found`,
	}, {
		description: "test valid cloud with invalid region fails",
		args:        []string{"--region", "dummy/invalidRegion", "one=two"},
		errorMatch:  `invalid region specified: "invalidRegion"`,
	}, {
		description: "test no cloud with invalid region fails",
		args:        []string{"--region", "invalidRegion", "one=two"},
		errorMatch:  `invalid region specified: "invalidRegion"`,
	}, {
		description: "test valid region with set arg succeeds",
		args:        []string{"--region", "dummy-region", "one=two"},
		nilErr:      true,
	}, {
		description: "test valid cloud with set and reset",
		args:        []string{"--cloud", "dummy", "one=two", "--reset", "three"},
		nilErr:      true,
	}, {
		description: "test valid region with set and reset",
		args:        []string{"--region", "dummy-region", "one=two", "--reset", "three"},
		nilErr:      true,
	}, {
		description: "test reset and set with valid region and extra key fails",
		args:        []string{"--reset", "something,else", "--region", "dummy-region", "invalidkey", "is=weird"},
		errorMatch:  "cannot use --reset flag, get value and set key=value pairs simultaneously",
	}, {
		// test various invalid
		description: "test too many positional args with reset",
		args:        []string{"--reset", "a", "b", "c", "d"},
		errorMatch:  "cannot use --reset flag and get value simultaneously",
	}, {
		description: "test too many positional args with invalid region set",
		args:        []string{"--region", "a", "a=b", "b", "c=d"},
		errorMatch:  "cannot get value and set key=value pairs simultaneously",
	}, {
		description: "test invalid positional args with set",
		args:        []string{"a=b", "b", "c=d"},
		errorMatch:  "cannot get value and set key=value pairs simultaneously",
	}, {
		description: "test invalid positional args with set and trailing key",
		args:        []string{"a=b", "c=d", "e"},
		errorMatch:  "cannot get value and set key=value pairs simultaneously",
	}, {
		description: "test invalid positional args with valid region, set, reset",
		args:        []string{"--region", "dummy-region", "a=b", "--reset", "c,d,", "e=f", "g"},
		errorMatch:  "cannot use --reset flag, get value and set key=value pairs simultaneously",
	}, {
		// Test some random orderings
		description: "test region set and split key=values",
		args:        []string{"--region", "dummy-region", "a=b", "--reset", "c,d,", "e=f"},
		nilErr:      true,
	}, {
		description: "test leading comma with reset",
		args:        []string{"--reset", ",a,b"},
		nilErr:      true,
	}} {
		c.Logf("test %d: %s", i, test.description)
		_, err := s.run(c, test.args...)
		if test.nilErr {
			c.Check(err, tc.ErrorIsNil)
			continue
		}
		c.Check(err, tc.ErrorMatches, test.errorMatch)
	}
}

func (s *DefaultsCommandSuite) TestMultiCloudMessage(c *tc.C) {
	s.fakeCloudAPI.clouds[names.NewCloudTag("another")] = cloud.Cloud{Name: "another"}
	_, err := s.run(c, "attr")
	c.Assert(err, tc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, tc.Matches, "You haven't specified a cloud and more than one exists on this controller.*another,dummy")
}

func (s *DefaultsCommandSuite) TestNoVisibleCloudMessage(c *tc.C) {
	s.fakeCloudAPI.clouds = nil
	_, err := s.run(c, "attr")
	c.Assert(err, tc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, tc.Matches, "You don't have access to any clouds on this controller.Only controller administrators can set default model values.")
}

func (s *DefaultsCommandSuite) TestResetUnknownValueLogs(c *tc.C) {
	ctx, err := s.run(c, "--reset", "attr,weird")
	c.Assert(err, tc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), tc.Contains, expected)
	c.Check(cmdtesting.Stdout(ctx), tc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestResetAttr(c *tc.C) {
	ctx, err := s.run(c, "--reset", "attr,unknown")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, tc.Equals, "dummy")
	c.Check(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
	})
	c.Check(cmdtesting.Stdout(ctx), tc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestResetRegionAttr(c *tc.C) {
	ctx, err := s.run(c, "--reset", "attr,unknown", "--region", "dummy-region")
	c.Check(err, tc.ErrorIsNil)
	c.Check(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
	})
	c.Check(cmdtesting.Stdout(ctx), tc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestResetBlockedError(c *tc.C) {
	s.fakeDefaultsAPI.err = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "attr")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *DefaultsCommandSuite) TestSetUnknownValueLogs(c *tc.C) {
	_, err := s.run(c, "weird=foo")
	c.Assert(err, tc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), tc.Contains, expected)
}

func (s *DefaultsCommandSuite) TestSet(c *tc.C) {
	_, err := s.run(c, "special=extra", "attr=baz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, tc.Equals, "dummy")
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
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

func (s *DefaultsCommandSuite) TestSetReset(c *tc.C) {
	ctx, err := s.run(c, "special=extra", "--reset", "attr,unknown")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, tc.Equals, "dummy")
	c.Check(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"special": {Controller: "extra", Default: nil, Regions: nil},
	})
	c.Check(cmdtesting.Stdout(ctx), tc.DeepEquals, "")
}

func (s *DefaultsCommandSuite) TestSetValueWithSlash(c *tc.C) {
	// A value with a "/" might be interpreted as a cloud/region.
	_, err := s.run(c, `juju-no-proxy=localhost,127.0.0.1,127.0.0.53,10.0.8.0/24`)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.cloud, tc.Equals, "dummy")
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
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

func (s *DefaultsCommandSuite) TestSetFromFile(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("special: extra\n"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
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

func (s *DefaultsCommandSuite) TestSetFromStdin(c *tc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("special: extra\n")
	code := cmd.Main(model.NewDefaultsCommandForTest(
		s.fakeAPIRoot, s.fakeDefaultsAPI, s.fakeCloudAPI, s.store), ctx,
		[]string{"--file", "-"})

	c.Assert(code, tc.Equals, 0)
	output := strings.TrimSpace(cmdtesting.Stdout(ctx))
	c.Assert(output, tc.Equals, "")
	stderr := strings.TrimSpace(cmdtesting.Stderr(ctx))
	c.Assert(stderr, tc.Equals, "")

	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
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

func (s *DefaultsCommandSuite) TestSetFromFileCombined(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`
special: extra
attr: foo`), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile, "attr=baz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
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

func (s *DefaultsCommandSuite) TestSetFromFileReset(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`
special: extra
attr: foo`), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile, "--reset", "attr")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
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

func (s *DefaultsCommandSuite) TestSetFromFileUsingYAML(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`
special:
  default: meshuggah
`), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.run(c, "attr=baz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: "baz", Default: nil, Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"special": {Controller: "meshuggah", Default: nil, Regions: nil},
	})
}

func (s *DefaultsCommandSuite) TestSetFromFileUsingYAMLTargettingController(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`
special:
  default: meshuggah
  controller: nadir
`), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.run(c, "attr=baz")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: "baz", Default: nil, Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}, {
			Name:  "another-region",
			Value: "another-value",
		}}},
		"special": {Controller: "nadir", Default: nil, Regions: nil},
	})
}

func (s *DefaultsCommandSuite) TestSetFromFileUsingYAMLTargettingCloudRegion(c *tc.C) {
	table := []struct {
		input, cloud, region string
	}{
		{"dummy-region", "dummy", "dummy-region"},
		{"dummy/dummy-region", "dummy", "dummy-region"},
		{"another-region", "dummy", "another-region"},
	}
	for i, test := range table {
		c.Logf("test %d", i)
		tmpdir := c.MkDir()
		configFile := filepath.Join(tmpdir, "config.yaml")
		err := os.WriteFile(configFile, []byte(`
special:
  default: meshuggah
  controller: nadir
  regions:
  - name: `+test.region+`
    value: zenith
`), 0644)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.run(c, "--region", test.input, "--file", configFile)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(s.fakeDefaultsAPI.defaults, tc.DeepEquals, config.ModelDefaultAttributes{
			"attr": {Controller: nil, Default: "foo", Regions: nil},
			"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
				Name:  "dummy-region",
				Value: "dummy-value",
			}, {
				Name:  "another-region",
				Value: "another-value",
			}}},
			"special": {Controller: nil, Default: nil, Regions: []config.RegionDefaultValue{{
				Name:  test.region,
				Value: "zenith",
			}}},
		})
	}
}

func (s *DefaultsCommandSuite) TestSetConveysCloudRegion(c *tc.C) {
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
			_, err = s.run(c, "special=extra")
		} else {
			_, err = s.run(c, "--region", test.input, "special=extra")
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(s.fakeDefaultsAPI.region, tc.DeepEquals, test.region)
		c.Assert(s.fakeDefaultsAPI.cloud, tc.DeepEquals, test.cloud)
	}
}

func (s *DefaultsCommandSuite) TestBlockedErrorOnSet(c *tc.C) {
	s.fakeDefaultsAPI.err = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *DefaultsCommandSuite) TestGetSingleValue(c *tc.C) {
	context, err := s.run(c, "attr2")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fakeDefaultsAPI.cloud, tc.Equals, "dummy")
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"Attribute         Default        Controller\n" +
		"attr2             -              bar\n" +
		"  dummy-region    dummy-value    -\n" +
		"  another-region  another-value  -"
	c.Assert(output, tc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetSingleValueJSON(c *tc.C) {
	context, err := s.run(c, "--format=json", "attr2")
	c.Assert(err, tc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(output, tc.Equals,
		`{"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"},{"name":"another-region","value":"another-value"}]}}`)
}

func (s *DefaultsCommandSuite) TestGetAllValuesYAML(c *tc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(output, tc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetAllValuesJSON(c *tc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, tc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := `{"attr":{"default":"foo"},"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"},{"name":"another-region","value":"another-value"}]}}`
	c.Assert(output, tc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetAllValuesTabular(c *tc.C) {
	context, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"Attribute         Default        Controller\n" +
		"attr              foo            -\n" +
		"attr2             -              bar\n" +
		"  dummy-region    dummy-value    -\n" +
		"  another-region  another-value  -"
	c.Assert(output, tc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetRegionValuesTabular(c *tc.C) {
	context, err := s.run(c, "--region", "dummy-region")
	c.Assert(err, tc.ErrorIsNil)

	output := strings.TrimSpace(cmdtesting.Stdout(context))
	expected := "" +
		"Attribute       Default      Controller\n" +
		"attr2           -            bar\n" +
		"  dummy-region  dummy-value  -"
	c.Assert(output, tc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetRegionNoValuesTabular(c *tc.C) {
	_, err := s.run(c, "--reset", "attr2")
	c.Assert(err, tc.ErrorIsNil)
	ctx, err := s.run(c, "--region", "dummy-region")
	c.Assert(err, tc.ErrorMatches, `there are no default model values in region "dummy-region"`)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *DefaultsCommandSuite) TestGetRegionOneArgNoValuesTabular(c *tc.C) {
	ctx, err := s.run(c, "--region", "dummy-region", "attr")
	c.Assert(err, tc.ErrorMatches, `there are no default model values for "attr" in region "dummy-region"`)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *DefaultsCommandSuite) TestCloudRegion(c *tc.C) {
	// Test different ways of specifying cloud/region
	tests := []struct {
		about              string
		args               []string
		cloud, region, err string
	}{{
		about: "no cloud/region specified",
		args:  []string{},
		cloud: "dummy",
	}, {
		about: "--cloud=<cloud>",
		args:  []string{"--cloud", "dummy"},
		cloud: "dummy",
	}, {
		about:  "--region=<region>",
		args:   []string{"--region", "dummy-region"},
		cloud:  "dummy",
		region: "dummy-region",
	}, {
		about:  "--cloud=<cloud> --region=<region>",
		args:   []string{"--cloud", "dummy", "--region", "dummy-region"},
		cloud:  "dummy",
		region: "dummy-region",
	}, {
		about:  "--region=<cloud>/<region>",
		args:   []string{"--region", "dummy/dummy-region"},
		cloud:  "dummy",
		region: "dummy-region",
	}, {
		about: "--cloud=<cloud> --region=<cloud>/<region>",
		args:  []string{"--cloud", "dummy", "--region", "dummy/dummy-region"},
		err:   "(?m)cannot specify cloud using both --cloud and --region flags.*",
	}}

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)
		_, err := s.run(c, append(t.args, "foo=bar")...)
		if t.err == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(s.fakeDefaultsAPI.cloud, tc.Equals, t.cloud)
			c.Check(s.fakeDefaultsAPI.region, tc.Equals, t.region)
		} else {
			c.Assert(err, tc.ErrorMatches, t.err)
		}
	}
}
