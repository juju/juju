// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	goyaml "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
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
	_ = tc.Suite(&configCommandSuite{})

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
				"trust": map[string]interface{}{
					"description": "Specifies trust",
					"type":        "bool",
					"value":       true,
				},
			},
			"settings": charmSettings,
		},
	}, {
		"dummy-application",
		false,
		map[string]interface{}{
			"application": "dummy-application",
			"charm":       "dummy",
			"settings":    charmSettings,
		},
	},
}

func (s *configCommandSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.defaultCharmValues = map[string]interface{}{
		"title":           "Nearly There",
		"skill-level":     100,
		"username":        "admin001",
		"outlook":         "true",
		"multiline-value": "The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" ",
	}
	s.defaultAppValues = map[string]interface{}{
		"trust": true,
	}

	s.fake = &fakeApplicationAPI{
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

func (s *configCommandSuite) TestGetCommandInit(c *tc.C) {
	// missing args
	err := cmdtesting.InitCommand(application.NewConfigCommandForTest(s.fake, s.store), []string{})
	c.Assert(err, tc.ErrorMatches, "no application name specified")
}

func (s *configCommandSuite) TestGetCommandInitWithApplication(c *tc.C) {
	err := cmdtesting.InitCommand(application.NewConfigCommandForTest(s.fake, s.store), []string{"app"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetCommandInitWithKey(c *tc.C) {
	err := cmdtesting.InitCommand(application.NewConfigCommandForTest(s.fake, s.store), []string{"app", "key"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetConfig(c *tc.C) {
	for _, t := range getTests {
		if !t.useAppConfig {
			s.fake.appValues = nil
		}
		ctx := cmdtesting.Context(c)
		code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{t.application})
		c.Check(code, tc.Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")

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

func (s *configCommandSuite) TestGetCharmConfigKey(c *tc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{"dummy-application", "title"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "Nearly There\n")
}

func (s *configCommandSuite) TestGetCharmConfigKeyMultilineValue(c *tc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{"dummy-application", "multiline-value"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx),
		tc.Equals,
		"The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" \n")
}

func (s *configCommandSuite) TestGetCharmConfigKeyMultilineValueJSON(c *tc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{"dummy-application", "multiline-value", "--format", "json"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx),
		tc.Equals,
		"The quick brown fox jumps over the lazy dog. \"The quick brown fox jumps over the lazy dog\" \"The quick brown fox jumps over the lazy dog\" \n",
	)
}

func (s *configCommandSuite) TestGetAppConfigKey(c *tc.C) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(
		s.fake, s.store), ctx, []string{"dummy-application", "trust"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "true\n")
}

func (s *configCommandSuite) TestGetConfigKeyNotFound(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, application.NewConfigCommandForTest(s.fake, s.store), "dummy-application", "invalid")
	c.Assert(err, tc.ErrorMatches, `key "invalid" not found in "dummy-application" application config or charm settings.`, tc.Commentf("details: %v", errors.Details(err)))
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
	about:       "set and reset same key",
	args:        []string{"application", "key=val", "--reset", "key"},
	expectError: `cannot set and reset key "key" simultaneously`,
}}

func (s *configCommandSuite) TestSetCommandInitError(c *tc.C) {
	testStore := jujuclienttesting.MinimalStore()
	for i, test := range setCommandInitErrorTests {
		c.Logf("test %d: %s", i, test.about)
		cmd := application.NewConfigCommandForTest(s.fake, s.store)
		cmd.SetClientStore(testStore)
		err := cmdtesting.InitCommand(cmd, test.args)
		c.Check(err, tc.ErrorMatches, test.expectError)
	}
}

func (s *configCommandSuite) TestSetCharmConfigSuccess(c *tc.C) {
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
}

func (s *configCommandSuite) TestSetAppConfigSuccess(c *tc.C) {
	s.assertSetSuccess(c, s.dir, []string{
		"trust=false",
	}, map[string]interface{}{
		"trust": "false",
	}, s.defaultCharmValues)
	s.assertSetSuccess(c, s.dir, []string{
		"trust=true",
	}, map[string]interface{}{
		"trust": "true",
	}, s.defaultCharmValues)
}

func (s *configCommandSuite) TestSetSameValue(c *tc.C) {
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

func (s *configCommandSuite) TestSetConfigFail(c *tc.C) {
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

func (s *configCommandSuite) TestSetCharmConfigFromYAML(c *tc.C) {
	s.assertSetFail(c, s.dir, []string{
		"--file",
		"missing.yaml",
	}, ".*"+utils.NoSuchFileErrRegexp)

	ctx := cmdtesting.ContextForDir(c, s.dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{
		"dummy-application",
		"--file",
		"testconfig.yaml"})

	c.Check(code, tc.Equals, 0)
	c.Check(s.fake.config, tc.Equals, yamlConfigValue)
}

func (s *configCommandSuite) TestSetFromStdin(c *tc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("settings:\n  username:\n  value: world\n")
	code := cmd.Main(application.NewConfigCommandForTest(s.fake, s.store), ctx, []string{
		"dummy-application",
		"--file",
		"-"})
	c.Check(code, tc.Equals, 0)
	c.Check(s.fake.config, jc.DeepEquals, "settings:\n  username:\n  value: world\n")
}

func (s *configCommandSuite) TestResetCharmConfigToDefault(c *tc.C) {
	s.fake = &fakeApplicationAPI{
		name: "dummy-application", charmValues: map[string]interface{}{
			"username": "hello",
		}}
	s.assertResetSuccess(c, s.dir, []string{
		"--reset",
		"username",
	}, nil, make(map[string]interface{}))
}

func (s *configCommandSuite) TestResetAppConfig(c *tc.C) {
	s.fake = &fakeApplicationAPI{
		name: "dummy-application", appValues: map[string]interface{}{
			"trust": false,
		}}
	s.assertResetSuccess(c, s.dir, []string{
		"--reset",
		"trust",
	}, make(map[string]interface{}), nil)
}

func (s *configCommandSuite) TestBlockSetConfig(c *tc.C) {
	// Block operation
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockSetConfig")
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommandInDir(c, cmd, []string{
		"dummy-application",
		"--file",
		"testconfig.yaml",
	}, s.dir)
	c.Assert(err, tc.ErrorMatches, `(.|\n)*All operations that change model have been disabled(.|\n)*`)
	c.Check(c.GetTestLog(), tc.Matches, "(.|\n)*TestBlockSetConfig(.|\n)*")
}

func (s *configCommandSuite) TestSetReset(c *tc.C) {
	s.fake = &fakeApplicationAPI{
		name: "dummy-application", appValues: map[string]interface{}{
			"trust": false,
		}}
	s.assertResetSuccess(c, s.dir, []string{
		"username=foo",
		"--reset",
		"trust",
	}, make(map[string]interface{}), map[string]interface{}{"username": "foo"})
}

func (s *configCommandSuite) TestSetYAML(c *tc.C) {
	ctx := cmdtesting.ContextForDir(c, s.dir)
	api := &parseYamlAPI{*s.fake}
	code := cmd.Main(application.NewConfigCommandForTest(api, s.store), ctx, []string{
		"dummy-application", "--file", "testconfig.yaml"})
	c.Assert(code, tc.Equals, 0)
	c.Check(api.charmValues["skill-level"], tc.Equals, "9000")
	c.Check(api.charmValues["username"], tc.Equals, "admin002")
}

func (s *configCommandSuite) TestSetYAMLOverrideSet(c *tc.C) {
	ctx := cmdtesting.ContextForDir(c, s.dir)
	api := &parseYamlAPI{*s.fake}
	code := cmd.Main(application.NewConfigCommandForTest(api, s.store), ctx, []string{
		"dummy-application", "--file", "testconfig.yaml", "username=foo"})
	c.Assert(code, tc.Equals, 0)
	c.Check(api.charmValues["skill-level"], tc.Equals, "9000")
	c.Check(api.charmValues["username"], tc.Equals, "foo")
}

func (s *configCommandSuite) TestSetYAMLOverrideReset(c *tc.C) {
	ctx := cmdtesting.ContextForDir(c, s.dir)
	api := &parseYamlAPI{*s.fake}
	code := cmd.Main(application.NewConfigCommandForTest(api, s.store), ctx, []string{
		"dummy-application", "--file", "testconfig.yaml", "--reset", "skill-level"})
	c.Assert(code, tc.Equals, 0)
	c.Check(api.charmValues["skill-level"], tc.Equals, nil)
	c.Check(api.charmValues["username"], tc.Equals, "admin002")
}

// assertSetSuccess sets configuration options and checks the expected settings.
func (s *configCommandSuite) assertSetSuccess(
	c *tc.C, dir string, args []string,
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
	c *tc.C, dir string, args []string,
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
func (s *configCommandSuite) assertSetFail(c *tc.C, dir string, args []string, expectErr string) {
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())

	args = append([]string{"dummy-application"}, args...)
	_, err := cmdtesting.RunCommandInDir(c, cmd, args, dir)
	c.Assert(err, tc.ErrorMatches, expectErr)
}

func (s *configCommandSuite) assertNoWarning(c *tc.C, dir string, args []string) {
	cmd := application.NewConfigCommandForTest(s.fake, s.store)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommandInDir(c, cmd, append([]string{"dummy-application"}, args...), dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.Replace(c.GetTestLog(), "\n", " ", -1), tc.Not(tc.Matches), ".*WARNING.*")
}

// setupValueFile creates a file containing one value for testing
// set with name=@filename.
func setupValueFile(c *tc.C, dir, filename, value string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := os.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

// setupBigFile creates a too big file for testing
// set with name=@filename.
func setupBigFile(c *tc.C, dir string) string {
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
func setupConfigFile(c *tc.C, dir string) string {
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

func (f *parseYamlAPI) SetConfig(ctx context.Context, application, configYAML string, config map[string]string) error {
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
