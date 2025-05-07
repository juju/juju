// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type ConfigCommandSuite struct {
	fakeEnvSuite
}

var _ = tc.Suite(&ConfigCommandSuite{})

func (s *ConfigCommandSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	command := model.NewConfigCommandForTest(s.fake)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *ConfigCommandSuite) TestInit(c *tc.C) {
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
			desc:       "cannot reset and retrieve at the same time",
			args:       []string{"--reset", "something", "weird"},
			errorMatch: "cannot use --reset flag and get value simultaneously",
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
			errorMatch: "cannot specify multiple keys to get",
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
			c.Check(err, tc.ErrorIsNil)
			continue
		}
		c.Check(err, tc.ErrorMatches, test.errorMatch)
	}
}

func (s *ConfigCommandSuite) TestSingleValue(c *tc.C) {
	s.fake.values["special"] = "multi\nline"

	context, err := s.run(c, "special")
	c.Assert(err, tc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	c.Assert(output, tc.Equals, "multi\nline\n")
}

func (s *ConfigCommandSuite) TestSingleValueOutputFile(c *tc.C) {
	s.fake.values["special"] = "multi\nline"

	outpath := filepath.Join(c.MkDir(), "out")
	_, err := s.run(c, "--output", outpath, "special")
	c.Assert(err, tc.ErrorIsNil)

	output, err := os.ReadFile(outpath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(output), tc.Equals, "multi\nline\n")
}

func (s *ConfigCommandSuite) TestGetUnknownValue(c *tc.C) {
	context, err := s.run(c, "unknown")
	c.Assert(err, tc.ErrorMatches,
		`"unknown" is not a key of the currently targeted model: "king/sword"`)

	output := cmdtesting.Stdout(context)
	c.Assert(output, tc.Equals, "")
}

func (s *ConfigCommandSuite) TestSetFileNotFound(c *tc.C) {
	context, err := s.run(c, "--file", "bundles/k8s-model-config.yaml")
	c.Assert(err, tc.ErrorMatches, "open .*/bundles/k8s-model-config.yaml: no such file or directory")

	output := cmdtesting.Stdout(context)
	c.Assert(output, tc.Equals, "")
}

func (s *ConfigCommandSuite) TestSingleValueJSON(c *tc.C) {
	context, err := s.run(c, "--format=json", "special")
	c.Assert(err, tc.ErrorIsNil)

	want := "{\"special\":{\"Value\":\"special value\",\"Source\":\"model\"}}\n"
	output := cmdtesting.Stdout(context)
	c.Assert(output, tc.Equals, want)
}

func (s *ConfigCommandSuite) TestSingleValueYAML(c *tc.C) {
	context, err := s.run(c, "--format=yaml", "special")
	c.Assert(err, tc.ErrorIsNil)

	want := "" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"

	output := cmdtesting.Stdout(context)
	c.Assert(output, tc.Equals, want)
}

func (s *ConfigCommandSuite) TestAllValuesYAML(c *tc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, tc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	expected := "" +
		"running:\n" +
		"  value: true\n" +
		"  source: model\n" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"
	c.Assert(output, tc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesJSON(c *tc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, tc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	expected := `{"running":{"Value":true,"Source":"model"},"special":{"Value":"special value","Source":"model"}}` + "\n"
	c.Assert(output, tc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesTabular(c *tc.C) {
	context, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)

	output := cmdtesting.Stdout(context)
	expected := "" +
		"Attribute  From   Value\n" +
		"running    model  true\n" +
		"special    model  special value\n"
	c.Assert(output, tc.Equals, expected)
}

func (s *ConfigCommandSuite) TestSetAgentVersion(c *tc.C) {
	_, err := s.run(c, "agent-version=2.0.0")
	c.Assert(err, tc.ErrorMatches, `"agent-version" must be set via "upgrade-model"`)
}

func (s *ConfigCommandSuite) TestSetCharmhubURL(c *tc.C) {
	_, err := s.run(c, "charmhub-url=http://meshuggah.rocks")
	c.Assert(err, tc.ErrorMatches, `"charmhub-url" must be set via "add-model"`)
}

func (s *ConfigCommandSuite) TestSetSecretBackend(c *tc.C) {
	s.fake.bestVersion = 4
	_, err := s.run(c, "secret-backend=myvault")
	c.Assert(err, tc.ErrorMatches, `"secret-backend" has been removed from model config, use the new command "model-secret-backend" instead`)
}

func (s *ConfigCommandSuite) TestGetSecretBackend(c *tc.C) {
	s.fake.bestVersion = 4
	_, err := s.run(c, "secret-backend")
	c.Assert(err, tc.ErrorMatches, `"secret-backend" has been removed from model config, use the new command "model-secret-backend" instead`)
}

func (s *ConfigCommandSuite) TestSetAndReset(c *tc.C) {
	_, err := s.run(c, "--reset", "special", "foo=bar")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fake.resetKeys, tc.DeepEquals, []string{"special"})
	c.Check(s.fake.values, tc.DeepEquals, map[string]interface{}{
		"name":    "test-model",
		"special": "special value",
		"running": true,
		"foo":     "bar"})
}

func (s *ConfigCommandSuite) TestSetAndResetSameKey(c *tc.C) {
	_, err := s.run(c, "--reset", "special", "special=bar")
	c.Assert(err, tc.ErrorMatches, `cannot set and reset key "special" simultaneously`)
}

func (s *ConfigCommandSuite) TestSetFromFile(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("special: extra\n"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile)
	c.Assert(err, tc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"name":    "test-model",
		"running": true,
	}
	c.Assert(s.fake.values, tc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSetFromStdin(c *tc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("special: extra\n")
	code := cmd.Main(model.NewConfigCommandForTest(s.fake), ctx,
		[]string{"--file", "-"})

	c.Assert(code, tc.Equals, 0)
	output := strings.TrimSpace(cmdtesting.Stdout(ctx))
	c.Assert(output, tc.Equals, "")
	stderr := strings.TrimSpace(cmdtesting.Stderr(ctx))
	c.Assert(stderr, tc.Equals, "")

	expected := map[string]interface{}{
		"special": "extra",
		"name":    "test-model",
		"running": true,
	}
	c.Assert(s.fake.values, tc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSetFromFileUsingYAML(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`
special:
  value: extra
  source: default
`), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile)
	c.Assert(err, tc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"name":    "test-model",
		"running": true,
	}
	c.Assert(s.fake.values, tc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSetFromFileCombined(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("special: extra\nunknown: bar"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile, "unknown=foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fake.values, tc.DeepEquals, map[string]interface{}{
		"special": "extra", "unknown": "foo",
		"name":    "test-model",
		"running": true,
	})
}

func (s *ConfigCommandSuite) TestSetFromFileCombinedReset(c *tc.C) {
	tmpdir := c.MkDir()
	configFile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("special: extra\nunknown: bar"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.run(c, "--file", configFile, "--reset", "special,name")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fake.values, tc.DeepEquals, map[string]interface{}{
		"special": "extra",
		"name":    "test-model",
		"running": true,
		"unknown": "bar",
	})
	c.Check(s.fake.resetKeys, tc.DeepEquals, []string{"special", "name"})
}

func (s *ConfigCommandSuite) TestPassesValues(c *tc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, tc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"unknown": "foo",
		"name":    "test-model",
		"running": true,
	}
	c.Assert(s.fake.values, tc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestPassesCloudInitUserDataLong(c *tc.C) {
	modelCfg, err := s.fake.ModelGet(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	modelCfg["cloudinit-userdata"] = "test data"
	err = s.fake.ModelSet(context.Background(), modelCfg)
	c.Assert(err, tc.ErrorIsNil)

	context, err := s.run(c, "cloudinit-userdata")
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	c.Assert(output, tc.Equals, "test data\n")

	context2, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	output2 := cmdtesting.Stdout(context2)
	expected2 := "" +
		"Attribute           From   Value\n" +
		"cloudinit-userdata  model  <value set, see juju model-config cloudinit-userdata>\n" +
		"running             model  true\n" +
		"special             model  special value\n"
	c.Assert(output2, tc.Equals, expected2)
}

func (s *ConfigCommandSuite) TestPassesCloudInitUserDataShort(c *tc.C) {
	modelCfg, err := s.fake.ModelGet(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	modelCfg["cloudinit-userdata"] = ""
	err = s.fake.ModelSet(context.Background(), modelCfg)
	c.Assert(err, tc.ErrorIsNil)

	context, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	expected := "" +
		"Attribute           From   Value\n" +
		"cloudinit-userdata  model  \"\"\n" +
		"running             model  true\n" +
		"special             model  special value\n"
	c.Assert(output, tc.Equals, expected)
}

func (s *ConfigCommandSuite) TestSettingUnknownValue(c *tc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, tc.ErrorIsNil)
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), tc.Contains, expected)
}

func (s *ConfigCommandSuite) TestBlockedError(c *tc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *ConfigCommandSuite) TestResetPassesValues(c *tc.C) {
	_, err := s.run(c, "--reset", "special,running")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fake.resetKeys, tc.DeepEquals, []string{"special", "running"})
}

func (s *ConfigCommandSuite) TestResettingUnKnownValue(c *tc.C) {
	_, err := s.run(c, "--reset", "unknown")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fake.resetKeys, tc.DeepEquals, []string{"unknown"})
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), tc.Contains, expected)
}

func (s *ConfigCommandSuite) TestResetBlockedError(c *tc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "special")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}
