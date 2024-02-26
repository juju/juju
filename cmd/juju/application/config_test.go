// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"bytes"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type configCommandSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	dir                string
	fake               *fakeApplicationAPI
	store              jujuclient.ClientStore
	defaultCharmValues map[string]interface{}
	defaultAppValues   map[string]interface{}
}

var (
	_ = gc.Suite(&configCommandSuite{})

	validSetTestValue   = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
	invalidSetTestValue = "a value with an invalid UTF-8 sequence: " + string([]byte{0xFF, 0xFF})
	yamlConfigValue     = "dummy-application:\n  skill-level: 9000\n  username: admin002\n\n"
)

var charmSettings = map[string]interface{}{
	"multiline-value": map[string]interface{}{
		"description": "Specifies multiline-value",
		"type":        "string",
		"value":       "The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" ",
	},
	"title": map[string]interface{}{
		"description": "Specifies title",
		"type":        "string",
		"value":       "Nearly There",
	},
	"skill-level": map[string]interface{}{
		"description": "Specifies skill-level",
		"value":       100,
		"type":        "int",
	},
	"username": map[string]interface{}{
		"description": "Specifies username",
		"type":        "string",
		"value":       "admin001",
	},
	"outlook": map[string]interface{}{
		"description": "Specifies outlook",
		"type":        "string",
		"value":       "true",
	},
}

var getTests = []struct {
	application  string
	useAppConfig bool
	expected     map[string]interface{}
}{
	{
		"dummy-application",
		true,
		map[string]interface{}{
			"application": "dummy-application",
			"charm":       "dummy",
			"application-config": map[string]interface{}{
				"juju-external-hostname": map[string]interface{}{
					"description": "Specifies juju-external-hostname",
					"type":        "string",
					"value":       "ext-host",
				},
			},
			"settings":                               charmSettings,
			"changes will be targeted to generation": interface{}(model.GenerationMaster),
		},
	}, {
		"dummy-application",
		false,
		map[string]interface{}{
			"application":                            "dummy-application",
			"charm":                                  "dummy",
			"settings":                               charmSettings,
			"changes will be targeted to generation": interface{}(model.GenerationMaster),
		},
	},
}

func (s *configCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Branches)

	s.defaultCharmValues = map[string]interface{}{
		"title":           "Nearly There",
		"skill-level":     100,
		"username":        "admin001",
		"outlook":         "true",
		"multiline-value": "The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" ",
	}
	s.defaultAppValues = map[string]interface{}{
		"juju-external-hostname": "ext-host",
	}

	s.fake = &fakeApplicationAPI{
		branchName:  model.GenerationMaster,
		name:        "dummy-application",
		charmName:   "dummy",
		charmValues: s.defaultCharmValues,
		appValues:   s.defaultAppValues,
	}

	s.store = jujuclienttesting.MinimalStore()

	s.dir = c.MkDir()
	c.Assert(utf8.ValidString(validSetTestValue), jc.IsTrue)
	c.Assert(utf8.ValidString(invalidSetTestValue), jc.IsFalse)
	setupValueFile(c, s.dir, "valid.txt", validSetTestValue)
	setupValueFile(c, s.dir, "invalid.txt", invalidSetTestValue)
	setupBigFile(c, s.dir)
	setupConfigFile(c, s.dir)
}

func (s *configCommandSuite) TestGetCommandInit(c *gc.C) {
	// missing args
	err := cmdtesting.InitCommand(application.NewConfigCommandForTest(s.fake, s.store), []string{})
	c.Assert(err, gc.ErrorMatches, "no application name specified")
}

func (s *configCommandSuite) TestGetCommandInitWithApplication(c *gc.C) {
	err := cmdtesting.InitCommand(application.NewConfigCommandForTest(s.fake, s.store), []string{"app"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetCommandInitWithKey(c *gc.C) {
	err := cmdtesting.InitCommand(application.NewConfigCommandForTest(s.fake, s.store), []string{"app", "key"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetCommandInitWithGeneration(c *gc.C) {
	err := cmdtesting.InitCommand(
		application.NewConfigCommandForTest(s.fake, s.store),
		[]string{"app", "key", "--branch", model.GenerationMaster},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetConfig(c *gc.C) {
	s.SetFeatureFlags(feature.Branches)
	for _, t := range getTests {
		if !t.useAppConfig {
			s.fake.appValues = nil
		}
		ctx := cmdtesting.Context(c)
		code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{t.application})
		c.Check(code, gc.Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")

		// round trip via goyaml to avoid being sucked into a quagmire of
		// map[interface{}]interface{} vs map[string]interface{}. This is
		// also required if we add json support to this command.
		buf, err := goyaml.Marshal(t.expected)
		c.Assert(err, jc.ErrorIsNil)
		expected := make(map[string]interface{})
		err = goyaml.Unmarshal(buf, &expected)
		c.Assert(err, jc.ErrorIsNil)

		actual := make(map[string]interface{})
		err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Log(ctx.Stdout.(*bytes.Buffer).String())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actual, jc.DeepEquals, expected)
	}
}

func (s *configCommandSuite) TestGetCharmConfigKey(c *gc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{"dummy-application", "title"})
	c.Check(code, gc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Nearly There\n")
}

func (s *configCommandSuite) TestGetCharmConfigKeyMultilineValue(c *gc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{"dummy-application", "multiline-value"})
	c.Check(code, gc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx),
		gc.Equals,
		"The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" \n")
}

func (s *configCommandSuite) TestGetCharmConfigKeyMultilineValueJSON(c *gc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{"dummy-application", "multiline-value", "--format", "json"})
	c.Check(code, gc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx),
		gc.Equals,
		"The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" \n",
	)
}

func (s *configCommandSuite) TestGetAppConfigKey(c *gc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(
		s.fake, s.store), ctx, []string{"dummy-application", "juju-external-hostname"})
	c.Check(code, gc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "ext-host\n")
}

func (s *configCommandSuite) TestGetConfigKeyNotFound(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, application.NewConfigCommandForTest(s.fake, s.store), "dummy-application", "invalid")
	c.Assert(err, gc.ErrorMatches, `key "invalid" not found in "dummy-application" application config or charm settings.`, gc.Commentf("details: %v", errors.Details(err)))
}

var setCommandInitErrorTests = []struct {
	about       string
	args        []string
	expectError string
}{{
	about:       "no arguments",
	expectError: "no application name specified",
}, {
	about:       "missing application name",
	args:        []string{"name=foo"},
	expectError: "no application name specified",
}, {
	about:       "--file path, but no application",
	args:        []string{"--file", "testconfig.yaml"},
	expectError: "no application name specified",
}, {
	about:       "--reset and no config name provided",
	args:        []string{"application", "--reset"},
	expectError: "option needs an argument: --reset",
}, {
	about:       "cannot set and retrieve simultaneously",
	args:        []string{"application", "get", "set=value"},
	expectError: "cannot get value and set key=value pairs simultaneously",
}, {
	about:       "cannot reset and get simultaneously",
	args:        []string{"application", "--reset", "reset", "get"},
	expectError: "cannot use --reset flag and get value simultaneously",
}, {
	about:       "invalid reset keys",
	args:        []string{"application", "--reset", "reset,bad=key"},
	expectError: `--reset accepts a comma delimited set of keys "a,b,c", received: "bad=key"`,
}, {
	about:       "init too many args fails",
	args:        []string{"application", "key", "another"},
	expectError: "cannot specify multiple keys to get",
}, {
	about:       "--branch with no value",
	args:        []string{"application", "key", "--branch"},
	expectError: "option needs an argument: --branch",
}, {
	about:       "set and reset same key",
	args:        []string{"application", "key=val", "--reset", "key"},
	expectError: `cannot set and reset key "key" simultaneously`,
}}

func (s *configCommandSuite) TestSetCommandInitError(c *gc.C) {
	testStore := jujuclienttesting.MinimalStore()
	for i, test := range setCommandInitErrorTests {
		c.Logf("test %d: %s", i, test.about)
		cmd := application.NewConfigCommandForTest(s.fake, s.store)
		cmd.SetClientStore(testStore)
		err := cmdtesting.InitCommand(cmd, test.args)
		c.Check(err, gc.ErrorMatches, test.expectError)
	}
}

func (s *configCommandSuite) TestSetCharmConfigSuccess(c *gc.C) {
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, s.defaultAppValues, map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello=foo",
	}, s.defaultAppValues, map[string]interface{}{
		"username": "hello=foo",
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"username=@valid.txt",
	}, s.defaultAppValues, map[string]interface{}{
		"username": validSetTestValue,
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"username=",
	}, s.defaultAppValues, map[string]interface{}{
		"username": "",
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"--branch",
		model.GenerationMaster,
		"username=hello",
		"outlook=hello@world.tld",
	}, s.defaultAppValues, map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})

	s.fake.branchName = "new-branch"
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello",
		"outlook=hello@world.tld",
		"--branch",
		"new-branch",
	}, s.defaultAppValues, map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})
}

func (s *configCommandSuite) TestSetAppConfigSuccess(c *gc.C) {
	s.assertSetSuccess(c, s.dir, []string{
		"juju-external-hostname=hello",
	}, map[string]interface{}{
		"juju-external-hostname": "hello",
	}, s.defaultCharmValues)
	s.assertSetSuccess(c, s.dir, []string{
		"juju-external-hostname=",
	}, map[string]interface{}{
		"juju-external-hostname": "",
	}, s.defaultCharmValues)
}

func (s *configCommandSuite) TestSetSameValue(c *gc.C) {
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, s.defaultAppValues, map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})
	s.assertNoWarning(c, s.dir, []string{"username=hello"})
	s.assertNoWarning(c, s.dir, []string{"outlook=hello@world.tld"})

}

func (s *configCommandSuite) TestSetConfigFail(c *gc.C) {
	s.assertSetFail(c, s.dir, []string{"foo", "bar"},
		"cannot specify multiple keys to get")
	s.assertSetFail(c, s.dir, []string{"=bar"}, "expected \"key=value\", got \"=bar\"")
	s.assertSetFail(c, s.dir, []string{
		"username=@missing.txt",
	}, "cannot read option from file \"missing.txt\": .* "+utils.NoSuchFileErrRegexp)
	s.assertSetFail(c, s.dir, []string{
		"username=@big.txt",
	}, "size of option file is larger than 5M")
	s.assertSetFail(c, s.dir, []string{
		"username=@invalid.txt",
	}, "value for option \"username\" contains non-UTF-8 sequences")
}

func (s *configCommandSuite) TestSetCharmConfigFromYAML(c *gc.C) {
	s.assertSetFail(c, s.dir, []string{
		"--file",
		"missing.yaml",
	}, ".*"+utils.NoSuchFileErrRegexp)

	ctx := cmdtesting.ContextForDir(c, s.dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{
		"dummy-application",
		"--file",
		"testconfig.yaml"})

	c.Check(code, gc.Equals, 0)
	c.Check(s.fake.config, gc.Equals, yamlConfigValue)
}

func (s *configCommandSuite) TestSetFromStdin(c *gc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("settings:\n  username:\n  value: world\n")
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{
		"dummy-application",
		"--file",
		"-"})
	c.Check(code, gc.Equals, 0)
	c.Check(s.fake.config, jc.DeepEquals, "settings:\n  username:\n  value: world\n")
}

func (s *configCommandSuite) TestResetCharmConfigToDefault(c *gc.C) {
	s.fake = &fakeApplicationAPI{
		branchName: model.GenerationMaster,
		name:       "dummy-application", charmValues: map[string]interface{}{
			"username": "hello",
		}}
	s.assertResetSuccess(c, s.dir, []string{
		"--reset",
		"username",
	}, nil, make(map[string]interface{}))
}

func (s *configCommandSuite) TestResetAppConfig(c *gc.C) {
	s.fake = &fakeApplicationAPI{
		branchName: model.GenerationMaster,
		name:       "dummy-application", appValues: map[string]interface{}{
			"juju-external-hostname": "app-value",
		}}
	s.assertResetSuccess(c, s.dir, []string{
		"--reset",
		"juju-external-hostname",
	}, make(map[string]interface{}), nil)
}

func (s *configCommandSuite) TestBlockSetConfig(c *gc.C) {
	// Block operation
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockSetConfig")
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommandInDir(c, cmd, []string{
		"dummy-application",
		"--file",
		"testconfig.yaml",
	}, s.dir)
	c.Assert(err, gc.ErrorMatches, `(.|\n)*All operations that change model have been disabled(.|\n)*`)
	c.Check(c.GetTestLog(), gc.Matches, "(.|\n)*TestBlockSetConfig(.|\n)*")
}

func (s *configCommandSuite) TestSetReset(c *gc.C) {
	s.fake = &fakeApplicationAPI{
		branchName: model.GenerationMaster,
		name:       "dummy-application", appValues: map[string]interface{}{
			"juju-external-hostname": "app-value",
		}}
	s.assertResetSuccess(c, s.dir, []string{
		"username=foo",
		"--reset",
		"juju-external-hostname",
	}, make(map[string]interface{}), map[string]interface{}{"username": "foo"})
}

func (s *configCommandSuite) TestSetYAML(c *gc.C) {
	ctx := cmdtesting.ContextForDir(c, s.dir)
	api := &parseYamlAPI{*s.fake}
	code := cmd.Main(application.NewConfigCommandForTest(api, s.store), ctx, []string{
		"dummy-application", "--file", "testconfig.yaml"})
	c.Assert(code, gc.Equals, 0)
	c.Check(api.charmValues["skill-level"], gc.Equals, "9000")
	c.Check(api.charmValues["username"], gc.Equals, "admin002")
}

func (s *configCommandSuite) TestSetYAMLOverrideSet(c *gc.C) {
	ctx := cmdtesting.ContextForDir(c, s.dir)
	api := &parseYamlAPI{*s.fake}
	code := cmd.Main(application.NewConfigCommandForTest(api, s.store), ctx, []string{
		"dummy-application", "--file", "testconfig.yaml", "username=foo"})
	c.Assert(code, gc.Equals, 0)
	c.Check(api.charmValues["skill-level"], gc.Equals, "9000")
	c.Check(api.charmValues["username"], gc.Equals, "foo")
}

func (s *configCommandSuite) TestSetYAMLOverrideReset(c *gc.C) {
	ctx := cmdtesting.ContextForDir(c, s.dir)
	api := &parseYamlAPI{*s.fake}
	code := cmd.Main(application.NewConfigCommandForTest(api, s.store), ctx, []string{
		"dummy-application", "--file", "testconfig.yaml", "--reset", "skill-level"})
	c.Assert(code, gc.Equals, 0)
	c.Check(api.charmValues["skill-level"], gc.Equals, nil)
	c.Check(api.charmValues["username"], gc.Equals, "admin002")
}

// assertSetSuccess sets configuration options and checks the expected settings.
func (s *configCommandSuite) assertSetSuccess(
	c *gc.C, dir string, args []string,
	expectAppValues map[string]interface{}, expectCharmValues map[string]interface{},
) {
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())

	args = append([]string{"dummy-application"}, args...)
	_, err := cmdtesting.RunCommandInDir(c, cmd, args, dir)
	c.Assert(err, jc.ErrorIsNil)
	appValues := make(map[string]interface{})
	for k, v := range s.defaultAppValues {
		appValues[k] = v
	}
	for k, v := range expectAppValues {
		appValues[k] = v
	}
	c.Assert(s.fake.appValues, jc.DeepEquals, appValues)

	charmValues := make(map[string]interface{})
	for k, v := range s.defaultCharmValues {
		charmValues[k] = v
	}
	for k, v := range expectCharmValues {
		charmValues[k] = v
	}
	c.Assert(s.fake.charmValues, jc.DeepEquals, charmValues)
}

func (s *configCommandSuite) assertResetSuccess(
	c *gc.C, dir string, args []string,
	expectAppValues map[string]interface{}, expectCharmValues map[string]interface{},
) {
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())

	args = append([]string{"dummy-application"}, args...)
	_, err := cmdtesting.RunCommandInDir(c, cmd, args, dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.appValues, jc.DeepEquals, expectAppValues)
	c.Assert(s.fake.charmValues, jc.DeepEquals, expectCharmValues)
}

// assertSetFail sets configuration options and checks the expected error.
func (s *configCommandSuite) assertSetFail(c *gc.C, dir string, args []string, expectErr string) {
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())

	args = append([]string{"dummy-application"}, args...)
	_, err := cmdtesting.RunCommandInDir(c, cmd, args, dir)
	c.Assert(err, gc.ErrorMatches, expectErr)
}

func (s *configCommandSuite) assertNoWarning(c *gc.C, dir string, args []string) {
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommandInDir(c, cmd, append([]string{"dummy-application"}, args...), dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.Replace(c.GetTestLog(), "\n", " ", -1), gc.Not(gc.Matches), ".*WARNING.*")
}

// setupValueFile creates a file containing one value for testing
// set with name=@filename.
func setupValueFile(c *gc.C, dir, filename, value string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := os.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

// setupBigFile creates a too big file for testing
// set with name=@filename.
func setupBigFile(c *gc.C, dir string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath("big.txt")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()
	chunk := make([]byte, 1024)
	for i := 0; i < cap(chunk); i++ {
		chunk[i] = byte(i % 256)
	}
	for i := 0; i < 6000; i++ {
		_, err = file.Write(chunk)
		c.Assert(err, jc.ErrorIsNil)
	}
	return path
}

// setupConfigFile creates a configuration file for testing set
// with the --file argument specifying a configuration file.
func setupConfigFile(c *gc.C, dir string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte(yamlConfigValue)
	err := os.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

// parseYamlAPI is a variant of fakeApplicationAPI which sets attributes from the provided YAML file.
type parseYamlAPI struct {
	fakeApplicationAPI
}

func (f *parseYamlAPI) SetConfig(branchName, application, configYAML string, config map[string]string) error {
	if branchName != f.branchName {
		return errors.Errorf("expected branch %q, got %q", f.branchName, branchName)
	}
	if f.err != nil {
		return f.err
	}

	if application != f.name {
		return errors.NotFoundf("application %q", application)
	}

	// Parse YAML into the config map
	parsed := map[string]map[string]string{}
	err := yaml.Unmarshal([]byte(configYAML), parsed)
	if err != nil {
		return err
	}
	for app, cfg := range parsed {
		if app == application {
			for key, val := range cfg {
				config[key] = val
			}
		}
	}

	if application != f.name {
		return errors.NotFoundf("application %q", application)
	}

	charmKeys := set.NewStrings("title", "skill-level", "username", "outlook")
	if f.charmValues == nil {
		f.charmValues = make(map[string]interface{})
	}
	for k, v := range config {
		if charmKeys.Contains(k) {
			f.charmValues[k] = v
		} else {
			f.appValues[k] = v
		}
	}

	return nil
}
