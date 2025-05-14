// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	stdtesting "testing"

	"github.com/juju/gnuflag"
	"github.com/juju/tc"

	jujucmd "github.com/juju/juju/internal/cmd"
)

type suite struct{}

var _ = tc.Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

func (s *suite) TestSetFlags(c *tc.C) {
	for _, resettable := range []bool{true, false} {
		cmd := ConfigCommandBase{Resettable: resettable}
		f := flagSetForTest(c)
		cmd.SetFlags(f)

		// Collect all flags
		expectedFlags := []string{"color", "file", "no-color"}
		if resettable {
			expectedFlags = append(expectedFlags, "reset")
		}

		flags := []string{}
		f.VisitAll(
			func(f *gnuflag.Flag) { flags = append(flags, f.Name) },
		)
		c.Check(flags, tc.SameContents, expectedFlags)

		c.Check(sliceContains(flags, "reset"), tc.Equals, resettable)
	}

}

// parseFailTest holds args for which we expect parsing to fail.
type parseFailTest struct {
	about      string
	resettable bool
	args       []string
	errMsg     string
}

// testParse checks that parsing of the given args fails or succeeds
// (depending on the value of `fail`).
func testParse(c *tc.C, test parseFailTest, fail bool) {
	cmd := ConfigCommandBase{Resettable: test.resettable}
	f := &gnuflag.FlagSet{}
	f.SetOutput(io.Discard)

	cmd.SetFlags(f)
	err := f.Parse(true, test.args)
	if fail {
		c.Assert(err, tc.ErrorMatches, test.errMsg)
	} else {
		c.Assert(err, tc.ErrorIsNil)
	}
}

// initFailTest represents a test for which we expect parsing to succeed, but
// initialisation to fail.
type initFailTest struct {
	about     string
	args      []string
	cantReset []string
	errMsg    string
}

// initTest represents a test for which we expect parsing and initialisation
// to succeed.
type initTest struct {
	about       string
	args        []string
	cantReset   []string
	actions     []Action
	configFile  jujucmd.FileVar
	keyToGet    string
	keysToReset []string
	valsToSet   Attrs
}

// setupInitTest sets up the ConfigCommandBase and error for TestInitFail and
// TestInitSucceed.
func setupInitTest(c *tc.C, args []string, cantReset []string) (ConfigCommandBase, error) {
	cmd := ConfigCommandBase{
		Resettable: true,
		CantReset:  cantReset,
	}
	f := flagSetForTest(c)
	cmd.SetFlags(f)
	err := f.Parse(true, args)
	c.Assert(err, tc.ErrorIsNil)
	err = cmd.Init(f.Args())

	return cmd, err
}

func (s *suite) TestParseFail(c *tc.C) {
	for i, test := range parseTests {
		c.Logf("test %d: %s", i, test.about)
		testParse(c, test, true)
	}
}

func (s *suite) TestInitFail(c *tc.C) {
	for i, test := range initFailTests {
		c.Logf("test %d: %s", i, test.about)
		// Check parsing succeeds
		testParse(c, parseFailTest{resettable: true, args: test.args}, false)

		_, err := setupInitTest(c, test.args, test.cantReset)
		c.Check(err, tc.ErrorMatches, test.errMsg)
	}
}

func (s *suite) TestInitSuccess(c *tc.C) {
	for i, test := range initTests {
		c.Logf("test %d: %s", i, test.about)
		// Check parsing succeeds
		testParse(c, parseFailTest{resettable: true, args: test.args}, false)

		cmd, err := setupInitTest(c, test.args, test.cantReset)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(cmd.Actions, tc.SameContents, test.actions)
		s.checkFileFirst(c, cmd.Actions)
		c.Check(cmd.ConfigFile, tc.DeepEquals, test.configFile)

		if sliceContains(cmd.Actions, GetOne) {
			c.Assert(cmd.KeysToGet, tc.HasLen, 1)
			c.Check(cmd.KeysToGet[0], tc.Equals, test.keyToGet)
		} else {
			c.Assert(cmd.KeysToGet, tc.HasLen, 0)
		}

		c.Check(cmd.KeysToReset, tc.DeepEquals, test.keysToReset)
		if test.valsToSet == nil {
			c.Check(cmd.ValsToSet, tc.HasLen, 0)
		} else {
			c.Check(cmd.ValsToSet, tc.DeepEquals, test.valsToSet)
		}
	}
}

var fileContents = []byte(`
key1: val1
key2: val2`)
var configAttrs = Attrs{
	"key1": "val1",
	"key2": "val2",
}

func (s *suite) TestReadFile(c *tc.C) {
	// Create file to read from
	dir := c.MkDir()
	filename := "cfg.yaml"
	err := os.WriteFile(path.Join(dir, filename), fileContents, 0666)
	c.Assert(err, tc.ErrorIsNil)

	cmd := ConfigCommandBase{
		ConfigFile: jujucmd.FileVar{Path: filename},
	}
	ctx := &jujucmd.Context{
		Context: c.Context(),
		Dir:     dir,
	}

	attrs, err := cmd.ReadFile(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(attrs, tc.DeepEquals, configAttrs)
}

func (s *suite) TestReadFileStdin(c *tc.C) {
	stdin := &bytes.Buffer{}
	stdin.Write(fileContents)

	cmd := ConfigCommandBase{
		ConfigFile: jujucmd.FileVar{Path: "-"},
	}
	ctx := &jujucmd.Context{
		Context: c.Context(),
		Stdin:   stdin,
	}

	attrs, err := cmd.ReadFile(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(attrs, tc.DeepEquals, configAttrs)
}

func (s *suite) TestReadNoSuchFile(c *tc.C) {
	// Create empty dir
	dir := c.MkDir()
	filename := "cfg.yaml"
	_, err := os.Stat(path.Join(dir, filename))
	c.Assert(err, tc.ErrorIs, fs.ErrNotExist)

	cmd := ConfigCommandBase{
		ConfigFile: jujucmd.FileVar{Path: filename},
	}
	ctx := &jujucmd.Context{
		Context: c.Context(),
		Dir:     dir,
	}

	_, err = cmd.ReadFile(ctx)
	c.Assert(err, tc.ErrorMatches, ".*no such file or directory")
}

func (s *suite) TestReadFileBadYAML(c *tc.C) {
	// Create file to read from
	dir := c.MkDir()
	filename := "cfg.yaml"
	err := os.WriteFile(path.Join(dir, filename), []byte("foo: foo: foo"), 0666)
	c.Assert(err, tc.ErrorIsNil)

	cmd := ConfigCommandBase{
		ConfigFile: jujucmd.FileVar{Path: filename},
	}
	ctx := &jujucmd.Context{
		Context: c.Context(),
		Dir:     dir,
	}

	_, err = cmd.ReadFile(ctx)
	c.Assert(err, tc.ErrorMatches, ".*yaml.*")
}

// checkFileFirst checks that if the provided list of Actions contains the
// SetFile action, then this is the first action in the list. This is important
// so that set/reset values from the command-line will override anything
// specified in a file.
func (s *suite) checkFileFirst(c *tc.C, actions []Action) {
	if sliceContains(actions, SetFile) {
		c.Check(actions[0], tc.Equals, SetFile)
	}
}

// flagSetForTest returns a flag set for running the parse/init tests.
func flagSetForTest(c *tc.C) *gnuflag.FlagSet {
	f := &gnuflag.FlagSet{
		Usage: func() { c.Fatalf("error occurred while parsing flags") },
	}
	return f
}

var parseTests = []parseFailTest{
	{
		about:  "no argument provided to --file",
		args:   []string{"--file"},
		errMsg: " needs an argument: --file",
	},
	{
		about:      "--reset when unresettable",
		resettable: false,
		args:       []string{"--reset", "key1"},
		errMsg:     " provided but not defined: --reset",
	},
	{
		about:      "no argument provided to --reset",
		resettable: true,
		args:       []string{"--reset"},
		errMsg:     " needs an argument: --reset",
	},
	{
		about:  "undefined flag --foo",
		args:   []string{"--foo"},
		errMsg: " provided but not defined: --foo",
	},
}

var initFailTests = []initFailTest{
	{
		about:  "get multiple keys",
		args:   []string{"key1", "key2"},
		errMsg: "cannot specify multiple keys to get",
	},
	{
		about:  "get and set",
		args:   []string{"key1", "key2=val2"},
		errMsg: "cannot get value and set key=value pairs simultaneously",
	},
	{
		about:  "set and get",
		args:   []string{"key1=val1", "key2"},
		errMsg: "cannot get value and set key=value pairs simultaneously",
	},
	{
		about:  "set and get multiple",
		args:   []string{"key1=val1", "key2", "key3", "key4=val4", "key5"},
		errMsg: "cannot get value and set key=value pairs simultaneously",
	},
	{
		about:  "get and reset",
		args:   []string{"key1", "--reset", "key2"},
		errMsg: "cannot use --reset flag and get value simultaneously",
	},
	{
		about:  "set and reset same key",
		args:   []string{"key1=val1", "--reset", "key1"},
		errMsg: `cannot set and reset key "key1" simultaneously`,
	},
	{
		about:  "get and set from file",
		args:   []string{"key1", "--file", "path"},
		errMsg: "cannot use --file flag and get value simultaneously",
	},
	{
		about:  "get, set from file & reset",
		args:   []string{"key1", "--file", "path", "--reset", "key1,key2"},
		errMsg: "cannot use --file flag, use --reset flag and get value simultaneously",
	},
	{
		about:  "set from file, set/reset same key",
		args:   []string{"key1=val1", "--file", "path", "--reset", "key1,key2"},
		errMsg: `cannot set and reset key "key1" simultaneously`,
	},
	{
		about:  "get, set, set from file and reset",
		args:   []string{"key1", "key2=val2", "--file", "path", "--reset", "key3,key4"},
		errMsg: "cannot use --file flag, use --reset flag, get value and set key=value pairs simultaneously",
	},
	{
		about:     "reset unresettable key",
		args:      []string{"--reset", "key1"},
		cantReset: []string{"key1"},
		errMsg:    `"key1" cannot be reset`,
	},
	{
		about:     "reset some unresettable keys",
		args:      []string{"--reset", "key1,key2,key3"},
		cantReset: []string{"key2", "key3"},
		errMsg:    `"key2" cannot be reset`,
	},
	{
		about:  "--reset with key=val",
		args:   []string{"--reset", "key1=val1"},
		errMsg: `--reset accepts a comma delimited set of keys "a,b,c", received: "key1=val1"`,
	},
	{
		about:  "--reset with some key=val",
		args:   []string{"--reset", "key1,key2=val2,key3"},
		errMsg: `--reset accepts a comma delimited set of keys "a,b,c", received: "key2=val2"`,
	},
	{
		about:  "--reset with multiple get keys",
		args:   []string{"--reset", "key1,key2,key3", "key4", "key5"},
		errMsg: "cannot use --reset flag and get value simultaneously",
	},
	{
		about:  "setting empty key is invalid",
		args:   []string{"=val"},
		errMsg: `expected "key=value", got "=val"`,
	},
}

var initTests = []initTest{
	{
		about:   "no args",
		args:    []string{},
		actions: []Action{GetAll},
	},
	{
		about:    "get single key",
		args:     []string{"key1"},
		actions:  []Action{GetOne},
		keyToGet: "key1",
	},
	{
		about:     "set key",
		args:      []string{"key1=val1"},
		actions:   []Action{SetArgs},
		valsToSet: Attrs{"key1": "val1"},
	},
	{
		about:   "set multiple keys",
		args:    []string{"key1=val1", "key2=val2", "key3=val3"},
		actions: []Action{SetArgs},
		valsToSet: Attrs{
			"key1": "val1",
			"key2": "val2",
			"key3": "val3",
		},
	},
	{
		about:       "reset key",
		args:        []string{"--reset", "key1"},
		actions:     []Action{Reset},
		keysToReset: []string{"key1"},
	},
	{
		about:       "reset multiple keys",
		args:        []string{"--reset", "key1,key2,key3"},
		actions:     []Action{Reset},
		keysToReset: []string{"key1", "key2", "key3"},
	},
	{
		about:      "set from file",
		args:       []string{"--file", "path"},
		actions:    []Action{SetFile},
		configFile: jujucmd.FileVar{Path: "path"},
	},
	{
		about:       "reset resettable key",
		args:        []string{"--reset", "key1"},
		cantReset:   []string{"key2"},
		actions:     []Action{Reset},
		keysToReset: []string{"key1"},
	},
	{
		about:       "reset resettable keys",
		args:        []string{"--reset", "key1,key2,key3"},
		cantReset:   []string{"key4", "key5"},
		actions:     []Action{Reset},
		keysToReset: []string{"key1", "key2", "key3"},
	},
	{
		about:   "set and reset",
		args:    []string{"key1=val1", "--reset", "key2"},
		actions: []Action{SetArgs, Reset},
		valsToSet: Attrs{
			"key1": "val1",
		},
		keysToReset: []string{"key2"},
	},
	{
		about:   "set and reset multiple",
		args:    []string{"key1=val1", "key2=val2", "--reset", "key3,key4"},
		actions: []Action{SetArgs, Reset},
		valsToSet: Attrs{
			"key1": "val1",
			"key2": "val2",
		},
		keysToReset: []string{"key3", "key4"},
	},
	{
		about:   "set multiple with multiple --reset",
		args:    []string{"key1=val1", "--reset", "key2", "key3=val3", "--reset", "key4"},
		actions: []Action{SetArgs, Reset},
		valsToSet: Attrs{
			"key1": "val1",
			"key3": "val3",
		},
		keysToReset: []string{"key2", "key4"},
	},
	{
		about:   "set and set from file",
		args:    []string{"key1=val1", "--file", "path"},
		actions: []Action{SetArgs, SetFile},
		valsToSet: Attrs{
			"key1": "val1",
		},
		configFile: jujucmd.FileVar{Path: "path"},
	},
	{
		about:       "set from file and reset",
		args:        []string{"--file", "path", "--reset", "key1,key2"},
		actions:     []Action{SetFile, Reset},
		configFile:  jujucmd.FileVar{Path: "path"},
		keysToReset: []string{"key1", "key2"},
	},
	{
		about:   "set from file, set and reset",
		args:    []string{"key1=val1", "--file", "path", "--reset", "key2,key3"},
		actions: []Action{SetFile, Reset, SetArgs},
		valsToSet: Attrs{
			"key1": "val1",
		},
		configFile:  jujucmd.FileVar{Path: "path"},
		keysToReset: []string{"key2", "key3"},
	},
	{
		about:      "read from stdin",
		args:       []string{"--file", "-"},
		actions:    []Action{SetFile},
		configFile: jujucmd.FileVar{Path: "-"},
	},
}
