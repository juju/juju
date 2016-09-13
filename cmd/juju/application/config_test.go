// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package application_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/application"
	coretesting "github.com/juju/juju/testing"
)

type configCommandSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	dir  string
	fake *fakeApplicationAPI
}

var (
	_ = gc.Suite(&configCommandSuite{})

	validSetTestValue   = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
	invalidSetTestValue = "a value with an invalid UTF-8 sequence: " + string([]byte{0xFF, 0xFF})
	yamlConfigValue     = "dummy-application:\n  skill-level: 9000\n  username: admin001\n\n"
)

var getTests = []struct {
	application string
	expected    map[string]interface{}
}{
	{
		"dummy-application",
		map[string]interface{}{
			"application": "dummy-application",
			"charm":       "dummy",
			"settings": map[string]interface{}{
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
			},
		},
	},
}

func (s *configCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeApplicationAPI{name: "dummy-application", charmName: "dummy",
		values: map[string]interface{}{
			"title":       "Nearly There",
			"skill-level": 100,
			"username":    "admin001",
			"outlook":     "true",
		}}
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

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
	err := coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{})
	c.Assert(err, gc.ErrorMatches, "no application name specified")
}

func (s *configCommandSuite) TestGetCommandInitWithApplication(c *gc.C) {
	err := coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"app"})
	// everything ok
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetCommandInitWithKey(c *gc.C) {
	err := coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"app", "key"})
	// everything ok
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configCommandSuite) TestGetCommandInitTooManyArgs(c *gc.C) {
	err := coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"app", "key", "another"})
	c.Assert(err, gc.ErrorMatches, "can only retrieve a single value, or all values")
}

func (s *configCommandSuite) TestGetConfig(c *gc.C) {
	for _, t := range getTests {
		ctx := coretesting.Context(c)
		code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, []string{t.application})
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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actual, gc.DeepEquals, expected)
	}
}

func (s *configCommandSuite) TestGetConfigKey(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, []string{"dummy-application", "title"})
	c.Check(code, gc.Equals, 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "Nearly There\n")
}

func (s *configCommandSuite) TestGetConfigKeyNotFound(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, []string{"dummy-application", "invalid"})
	c.Check(code, gc.Equals, 1)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "error: key \"invalid\" not found in \"dummy-application\" application settings.\n")
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *configCommandSuite) TestSetCommandInit(c *gc.C) {
	// missing args
	err := coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{})
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// missing application name
	err = coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"name=foo"})
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// --file path, but no application
	err = coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"--file", "testconfig.yaml"})
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// --file and options specified
	err = coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"application", "--file", "testconfig.yaml", "bees="})
	c.Assert(err, gc.ErrorMatches, "cannot specify --file and key=value arguments simultaneously")

	// --reset and no config name provided
	err = coretesting.InitCommand(application.NewConfigCommandForTest(s.fake), []string{"application", "--reset"})
	c.Assert(err, gc.ErrorMatches, "no configuration options specified")

}

func (s *configCommandSuite) TestSetOptionSuccess(c *gc.C) {
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello=foo",
	}, map[string]interface{}{
		"username": "hello=foo",
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"username=@valid.txt",
	}, map[string]interface{}{
		"username": validSetTestValue,
		"outlook":  "hello@world.tld",
	})
	s.assertSetSuccess(c, s.dir, []string{
		"username=",
	}, map[string]interface{}{
		"username": "",
		"outlook":  "hello@world.tld",
	})
}

func (s *configCommandSuite) TestSetSameValue(c *gc.C) {
	s.assertSetSuccess(c, s.dir, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, map[string]interface{}{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})
	s.assertSetWarning(c, s.dir, []string{
		"username=hello",
	}, "the configuration setting \"username\" already has the value \"hello\"")
	s.assertSetWarning(c, s.dir, []string{
		"outlook=hello@world.tld",
	}, "the configuration setting \"outlook\" already has the value \"hello@world.tld\"")

}

func (s *configCommandSuite) TestSetOptionFail(c *gc.C) {
	s.assertSetFail(c, s.dir, []string{"foo", "bar"},
		"error: can only retrieve a single value, or all values\n")
	s.assertSetFail(c, s.dir, []string{"=bar"}, "error: expected \"key=value\", got \"=bar\"\n")
	s.assertSetFail(c, s.dir, []string{
		"username=@missing.txt",
	}, "error: cannot read option from file \"missing.txt\": .* "+utils.NoSuchFileErrRegexp+"\n")
	s.assertSetFail(c, s.dir, []string{
		"username=@big.txt",
	}, "error: size of option file is larger than 5M\n")
	s.assertSetFail(c, s.dir, []string{
		"username=@invalid.txt",
	}, "error: value for option \"username\" contains non-UTF-8 sequences\n")
}

func (s *configCommandSuite) TestSetConfig(c *gc.C) {
	s.assertSetFail(c, s.dir, []string{
		"--file",
		"missing.yaml",
	}, "error.* "+utils.NoSuchFileErrRegexp+"\n")

	ctx := coretesting.ContextForDir(c, s.dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, []string{
		"dummy-application",
		"--file",
		"testconfig.yaml"})

	c.Check(code, gc.Equals, 0)
	c.Check(s.fake.config, gc.Equals, yamlConfigValue)
}

func (s *configCommandSuite) TestSetFromStdin(c *gc.C) {
	s.fake = &fakeApplicationAPI{name: "dummy-application"}
	ctx := coretesting.Context(c)
	ctx.Stdin = strings.NewReader("settings:\n  username:\n  value: world\n")
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, []string{
		"dummy-application",
		"--file",
		"-"})

	c.Check(code, gc.Equals, 0)
	c.Check(s.fake.config, jc.DeepEquals, "settings:\n  username:\n  value: world\n")
}

func (s *configCommandSuite) TestResetConfigToDefault(c *gc.C) {
	s.fake = &fakeApplicationAPI{name: "dummy-application", values: map[string]interface{}{
		"username": "hello",
	}}
	s.assertSetSuccess(c, s.dir, []string{
		"--reset",
		"username",
	}, make(map[string]interface{}))
}

func (s *configCommandSuite) TestBlockSetConfig(c *gc.C) {
	// Block operation
	s.fake.err = common.OperationBlockedError("TestBlockSetConfig")
	ctx := coretesting.ContextForDir(c, s.dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, []string{
		"dummy-application",
		"--file",
		"testconfig.yaml"})
	c.Check(code, gc.Equals, 1)
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockSetConfig.*")
}

// assertSetSuccess sets configuration options and checks the expected settings.
func (s *configCommandSuite) assertSetSuccess(c *gc.C, dir string, args []string, expect map[string]interface{}) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, append([]string{"dummy-application"}, args...))
	c.Assert(code, gc.Equals, 0)
}

// assertSetFail sets configuration options and checks the expected error.
func (s *configCommandSuite) assertSetFail(c *gc.C, dir string, args []string, err string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, append([]string{"dummy-application"}, args...))
	c.Check(code, gc.Not(gc.Equals), 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, err)
}

func (s *configCommandSuite) assertSetWarning(c *gc.C, dir string, args []string, w string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(application.NewConfigCommandForTest(s.fake), ctx, append([]string{"dummy-application"}, args...))
	c.Check(code, gc.Equals, 0)

	c.Assert(strings.Replace(c.GetTestLog(), "\n", " ", -1), gc.Matches, ".*WARNING.*"+w+".*")
}

// setupValueFile creates a file containing one value for testing
// set with name=@filename.
func setupValueFile(c *gc.C, dir, filename, value string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

// setupBigFile creates a too big file for testing
// set with name=@filename.
func setupBigFile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
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
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte(yamlConfigValue)
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}
