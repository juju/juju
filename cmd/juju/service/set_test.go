// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

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

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/service"
	coretesting "github.com/juju/juju/testing"
)

type SetSuite struct {
	coretesting.FakeJujuHomeSuite
	dir  string
	fake *fakeServiceAPI
}

var _ = gc.Suite(&SetSuite{})

var (
	validSetTestValue   = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
	invalidSetTestValue = "a value with an invalid UTF-8 sequence: " + string([]byte{0xFF, 0xFF})
	yamlConfigValue     = "dummy-service:\n  skill-level: 9000\n  username: admin001\n\n"
)

func (s *SetSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeServiceAPI{servName: "dummy-service", values: make(map[string]interface{})}

	s.dir = c.MkDir()
	c.Assert(utf8.ValidString(validSetTestValue), jc.IsTrue)
	c.Assert(utf8.ValidString(invalidSetTestValue), jc.IsFalse)
	setupValueFile(c, s.dir, "valid.txt", validSetTestValue)
	setupValueFile(c, s.dir, "invalid.txt", invalidSetTestValue)
	setupBigFile(c, s.dir)
	setupConfigFile(c, s.dir)
}

func (*SetSuite) TestSetCommandInit(c *gc.C) {
	// missing args
	err := coretesting.InitCommand(&service.SetCommand{}, []string{})
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// missing service name
	err = coretesting.InitCommand(&service.SetCommand{}, []string{"name=foo"})
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// --config path, but no service
	err = coretesting.InitCommand(&service.SetCommand{}, []string{"--config", "testconfig.yaml"})
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// --config and options specified
	err = coretesting.InitCommand(&service.SetCommand{}, []string{"service", "--config", "testconfig.yaml", "bees="})
	c.Assert(err, gc.ErrorMatches, "cannot specify --config when using key=value arguments")
}

func (s *SetSuite) TestSetOptionSuccess(c *gc.C) {
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

func (s *SetSuite) TestSetSameValue(c *gc.C) {
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

func (s *SetSuite) TestSetOptionFail(c *gc.C) {
	s.assertSetFail(c, s.dir, []string{"foo", "bar"}, "error: expected \"key=value\", got \"foo\"\n")
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

func (s *SetSuite) TestSetConfig(c *gc.C) {
	s.assertSetFail(c, s.dir, []string{
		"--config",
		"missing.yaml",
	}, "error.* "+utils.NoSuchFileErrRegexp+"\n")

	ctx := coretesting.ContextForDir(c, s.dir)
	code := cmd.Main(envcmd.Wrap(service.NewSetCommand(s.fake)), ctx, []string{
		"dummy-service",
		"--config",
		"testconfig.yaml"})

	c.Check(code, gc.Equals, 0)
	c.Check(s.fake.config, gc.Equals, yamlConfigValue)
}

func (s *SetSuite) TestBlockSetConfig(c *gc.C) {
	// Block operation
	s.fake.err = common.ErrOperationBlocked("TestBlockSetConfig")
	ctx := coretesting.ContextForDir(c, s.dir)
	code := cmd.Main(envcmd.Wrap(service.NewSetCommand(s.fake)), ctx, []string{
		"dummy-service",
		"--config",
		"testconfig.yaml"})
	c.Check(code, gc.Equals, 1)
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockSetConfig.*")
}

// assertSetSuccess sets configuration options and checks the expected settings.
func (s *SetSuite) assertSetSuccess(c *gc.C, dir string, args []string, expect map[string]interface{}) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(service.NewSetCommand(s.fake)), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Equals, 0)
	c.Assert(s.fake.values, gc.DeepEquals, expect)
}

// assertSetFail sets configuration options and checks the expected error.
func (s *SetSuite) assertSetFail(c *gc.C, dir string, args []string, err string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(service.NewSetCommand(s.fake)), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Not(gc.Equals), 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, err)
}

func (s *SetSuite) assertSetWarning(c *gc.C, dir string, args []string, w string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(service.NewSetCommand(s.fake)), ctx, append([]string{"dummy-service"}, args...))
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
// with the --config argument specifying a configuration file.
func setupConfigFile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte(yamlConfigValue)
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}
