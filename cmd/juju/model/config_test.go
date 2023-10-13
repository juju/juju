// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/testing"
)

type ConfigCommandSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&ConfigCommandSuite{})

func (s *ConfigCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewConfigCommandForTest(s.fake)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *ConfigCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		desc       string
		args       []string
		errorMatch string
		nilErr     bool
	}{
		{
			// Test reset
			desc:       "reset requires arg",
			args:       []string{"--reset"},
			errorMatch: "option needs an argument: --reset",
		}, {
			desc:       "cannot set and retrieve at the same time",
			args:       []string{"--reset", "something", "weird"},
			errorMatch: "cannot set and retrieve model values simultaneously",
		}, {
			desc:       "agent-version cannot be reset",
			args:       []string{"--reset", "agent-version"},
			errorMatch: `"agent-version" cannot be reset`,
		}, {
			desc:       "reset cannot have k=v pairs",
			args:       []string{"--reset", "a,b,c=d,e"},
			errorMatch: `--reset accepts a comma delimited set of keys "a,b,c", received: "c=d"`,
		}, {
			// Test get
			desc:   "get all succeeds",
			args:   nil,
			nilErr: true,
		}, {
			desc:   "get one succeeds",
			args:   []string{"one"},
			nilErr: true,
		}, {
			desc:       "get multiple fails",
			args:       []string{"one", "two"},
			errorMatch: `arg "one" is not a key-value pair or a filename`,
		}, {
			// test variations
			desc:   "test reset interspersed",
			args:   []string{"--reset", "one", "special=foo", "--reset", "two"},
			nilErr: true,
		},
	} {
		c.Logf("test %d: %s", i, test.desc)
		command := model.NewConfigCommandForTest(s.fake)
		err := cmdtesting.InitCommand(command, test.args)
		if test.nilErr {
			c.Check(err, jc.ErrorIsNil)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.errorMatch)
	}
}

func (s *ConfigCommandSuite) TestSingleValue(c *gc.C) {
	s.fake.values["special"] = "multi\nline"

	context, err := s.run(c, "special")
	c.Assert(err, jc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "multi\nline\n")
}

func (s *ConfigCommandSuite) TestSingleValueOutputFile(c *gc.C) {
	s.fake.values["special"] = "multi\nline"

	outpath := filepath.Join(c.MkDir(), "out")
	_, err := s.run(c, "--output", outpath, "special")
	c.Assert(err, jc.ErrorIsNil)

	output, err := ioutil.ReadFile(outpath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "multi\nline\n")
}

func (s *ConfigCommandSuite) TestGetUnknownValue(c *gc.C) {
	context, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches,
		`"unknown" seems to be neither a file nor a key of the currently targeted model: "king/sword"`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestGetProperErrorMessageOnPaths(c *gc.C) {
	context, err := s.run(c, "bundles/k8s-model-config.yaml")
	c.Assert(err, gc.ErrorMatches, `"bundles/k8s-model-config.yaml" seems to be a file but not found`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestGetFileLike(c *gc.C) {
	context, err := s.run(c, "bundles/k8s-model-config.json")
	c.Assert(err, gc.ErrorMatches, `"bundles/k8s-model-config.json" seems to be a file but not found`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestNotFileLike(c *gc.C) {
	context, err := s.run(c, "bundles/k8s-model-config--json")
	c.Assert(err, gc.ErrorMatches,
		`"bundles/k8s-model-config--json" seems to be neither a file nor a key of the currently targeted model: "king/sword"`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestNotFileLikeEndsWithDot(c *gc.C) {
	context, err := s.run(c, "bundles/k8s-model-config.")
	c.Assert(err, gc.ErrorMatches,
		`"bundles/k8s-model-config." seems to be neither a file nor a key of the currently targeted model: "king/sword"`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestNotFileLikeEndsTooLong(c *gc.C) {
	context, err := s.run(c, "bundles/k8s-model-config.fksdkfsd")
	c.Assert(err, gc.ErrorMatches,
		`"bundles/k8s-model-config.fksdkfsd" seems to be neither a file nor a key of the currently targeted model: "king/sword"`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "special")
	c.Assert(err, jc.ErrorIsNil)

	want := "{\"special\":{\"Value\":\"special value\",\"Source\":\"model\"}}\n"
	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *ConfigCommandSuite) TestSingleValueYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml", "special")
	c.Assert(err, jc.ErrorIsNil)

	want := "" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *ConfigCommandSuite) TestAllValuesYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	expected := "" +
		"running:\n" +
		"  value: true\n" +
		"  source: model\n" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	expected := `{"running":{"Value":true,"Source":"model"},"special":{"Value":"special value","Source":"model"}}` + "\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesTabular(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	expected := "" +
		"Attribute  From   Value\n" +
		"running    model  true\n" +
		"special    model  special value\n" +
		"\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestSetAgentVersion(c *gc.C) {
	_, err := s.run(c, "agent-version=2.0.0")
	c.Assert(err, gc.ErrorMatches, `"agent-version" must be set via "upgrade-model"`)
}

func (s *ConfigCommandSuite) TestSetCharmhubURL(c *gc.C) {
	_, err := s.run(c, "charmhub-url=http://meshuggah.rocks")
	c.Assert(err, gc.ErrorMatches, `"charmhub-url" must be set via "add-model"`)
}

func (s *ConfigCommandSuite) TestSetAndReset(c *gc.C) {
	_, err := s.run(c, "--reset", "special", "special=bar")
	c.Assert(err, gc.ErrorMatches, `key "special" cannot be both set and reset in the same command`)
}

func (s *ConfigCommandSuite) TestSetFromFile(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("special: extra\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, configFile)
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSetFromFileUsingYAML(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte(`
special:
  value: extra
  source: default
`), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, configFile)
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSetFromFileCombined(c *gc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("special: extra\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, configFile, "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"unknown": "foo",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestPassesValues(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"unknown": "foo",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestPassesCloudInitUserDataLong(c *gc.C) {
	modelCfg, err := s.fake.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	modelCfg["cloudinit-userdata"] = "test data"
	err = s.fake.ModelSet(modelCfg)
	c.Assert(err, jc.ErrorIsNil)

	context, err := s.run(c, "cloudinit-userdata")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, "test data\n")

	context2, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	output2 := cmdtesting.Stdout(context2)
	expected2 := "" +
		"Attribute           From   Value\n" +
		"cloudinit-userdata  model  <value set, see juju model-config cloudinit-userdata>\n" +
		"running             model  true\n" +
		"special             model  special value\n" +
		"\n"
	c.Assert(output2, gc.Equals, expected2)
}

func (s *ConfigCommandSuite) TestPassesCloudInitUserDataShort(c *gc.C) {
	modelCfg, err := s.fake.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	modelCfg["cloudinit-userdata"] = ""
	err = s.fake.ModelSet(modelCfg)
	c.Assert(err, jc.ErrorIsNil)

	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	expected := "" +
		"Attribute           From   Value\n" +
		"cloudinit-userdata  model  \"\"\n" +
		"running             model  true\n" +
		"special             model  special value\n" +
		"\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestSettingUnknownValue(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	// Command succeeds, but warning logged.
	//expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	//c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *ConfigCommandSuite) TestBlockedError(c *gc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *ConfigCommandSuite) TestResetPassesValues(c *gc.C) {
	_, err := s.run(c, "--reset", "special,running")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.resetKeys, jc.DeepEquals, []string{"special", "running"})
}

func (s *ConfigCommandSuite) TestResettingUnKnownValue(c *gc.C) {
	_, err := s.run(c, "--reset", "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.resetKeys, jc.DeepEquals, []string{"unknown"})
	// Command succeeds, but warning logged.
	//expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	//c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *ConfigCommandSuite) TestResetBlockedError(c *gc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "special")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}
