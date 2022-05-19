// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"bytes"
	stdtesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

func (s *suite) TestSetFlags(c *gc.C) {
	for _, resettable := range []bool{true, false} {
		cmd := ConfigCommandBase{Resettable: resettable}
		f := flagSetForTest(c)
		cmd.SetFlags(f)

		// Collect all flags
		expectedFlags := []string{"file"}
		if resettable {
			expectedFlags = append(expectedFlags, "reset")
		}

		flags := []string{}
		f.VisitAll(
			func(f *gnuflag.Flag) { flags = append(flags, f.Name) },
		)
		c.Check(flags, jc.SameContents, expectedFlags)

		c.Check(sliceContains(flags, "reset"), gc.Equals, resettable)
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
func testParse(c *gc.C, test parseFailTest, fail bool) {
	cmd := ConfigCommandBase{Resettable: test.resettable}
	parseFail := false
	var errWriter bytes.Buffer

	f := &gnuflag.FlagSet{
		Usage: func() { parseFail = true },
	}
	f.SetOutput(&errWriter)

	cmd.SetFlags(f)
	f.Parse(true, test.args)
	c.Assert(parseFail, gc.Equals, fail)
	if fail {
		c.Assert(errWriter.String(), gc.Equals, test.errMsg)
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
	action      ConfigAction
	configFile  cmd.FileVar
	keyToGet    string
	keysToReset []string
	valsToSet   map[string]string
}

// setupInitTest sets up the ConfigCommandBase and error for TestInitFail and
// TestInitSucceed.
func setupInitTest(c *gc.C, args []string, cantReset []string) (ConfigCommandBase, error) {
	cmd := ConfigCommandBase{
		Resettable: true,
		CantReset:  cantReset,
	}
	f := flagSetForTest(c)
	cmd.SetFlags(f)
	f.Parse(true, args)
	err := cmd.Init(f.Args())

	return cmd, err
}

func (s *suite) TestParseFail(c *gc.C) {
	for i, test := range parseTests {
		c.Logf("test %d: %s", i, test.about)
		testParse(c, test, true)
	}
}

func (s *suite) TestInitFail(c *gc.C) {
	for i, test := range initFailTests {
		c.Logf("test %d: %s", i, test.about)
		// Check parsing succeeds
		testParse(c, parseFailTest{resettable: true, args: test.args}, false)

		_, err := setupInitTest(c, test.args, test.cantReset)
		c.Check(err, gc.ErrorMatches, test.errMsg)
	}
}

func (s *suite) TestInitSuccess(c *gc.C) {
	for i, test := range initTests {
		c.Logf("test %d: %s", i, test.about)
		// Check parsing succeeds
		testParse(c, parseFailTest{resettable: true, args: test.args}, false)

		cmd, err := setupInitTest(c, test.args, test.cantReset)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(len(cmd.Actions), gc.Equals, 1)
		c.Check(cmd.Actions[0], gc.Equals, test.action)
		c.Check(cmd.ConfigFile, gc.DeepEquals, test.configFile)
		c.Check(cmd.KeyToGet, gc.DeepEquals, test.keyToGet)
		c.Check(cmd.KeysToReset, gc.DeepEquals, test.keysToReset)
		c.Check(cmd.ValsToSet, gc.DeepEquals, test.valsToSet)
	}
}

// flagSetForTest returns a flag set for running the parse/init tests.
func flagSetForTest(c *gc.C) *gnuflag.FlagSet {
	f := &gnuflag.FlagSet{
		Usage: func() { c.Fatalf("error occurred while parsing flags") },
	}
	return f
}

var parseTests = []parseFailTest{
	{
		about:  "no argument provided to --file",
		args:   []string{"--file"},
		errMsg: " needs an argument: --file\n",
	},
	{
		about:      "--reset when unresettable",
		resettable: false,
		args:       []string{"--reset", "key1"},
		errMsg:     " provided but not defined: --reset\n",
	},
	{
		about:      "no argument provided to --reset",
		resettable: true,
		args:       []string{"--reset"},
		errMsg:     " needs an argument: --reset\n",
	},
	{
		about:  "undefined flag --foo",
		args:   []string{"--foo"},
		errMsg: " provided but not defined: --foo\n",
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
		about:  "set and reset",
		args:   []string{"key1=val1", "--reset", "key2"},
		errMsg: "cannot use --reset flag and set key=value pairs simultaneously",
	},
	{
		about:  "set and reset same key",
		args:   []string{"key1=val1", "--reset", "key1"},
		errMsg: "cannot use --reset flag and set key=value pairs simultaneously",
	},
	{
		about:  "get and set from file",
		args:   []string{"key1", "--file", "path"},
		errMsg: "cannot use --file flag and get value simultaneously",
	},
	{
		about:  "set and set from file",
		args:   []string{"key1=val1", "--file", "path"},
		errMsg: "cannot use --file flag and set key=value pairs simultaneously",
	},
	{
		about:  "set from file and reset",
		args:   []string{"--file", "path", "--reset", "key1,key2"},
		errMsg: "cannot use --file flag and use --reset flag simultaneously",
	},
	{
		about:  "get, set from file & reset",
		args:   []string{"key1", "--file", "path", "--reset", "key1,key2"},
		errMsg: "cannot use --file flag, use --reset flag and get value simultaneously",
	},
	{
		about:  "set, set from file and reset",
		args:   []string{"key1=val1", "--file", "path", "--reset", "key1,key2"},
		errMsg: "cannot use --file flag, use --reset flag and set key=value pairs simultaneously",
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
}

var initTests = []initTest{
	{
		about:  "no args",
		args:   []string{},
		action: GetAll,
	},
	{
		about:    "get single key",
		args:     []string{"key1"},
		action:   GetOne,
		keyToGet: "key1",
	},
	{
		about:     "set key",
		args:      []string{"key1=val1"},
		action:    Set,
		valsToSet: map[string]string{"key1": "val1"},
	},
	{
		about:  "set multiple keys",
		args:   []string{"key1=val1", "key2=val2", "key3=val3"},
		action: Set,
		valsToSet: map[string]string{
			"key1": "val1",
			"key2": "val2",
			"key3": "val3",
		},
	},
	{
		about:       "reset key",
		args:        []string{"--reset", "key1"},
		action:      Reset,
		keysToReset: []string{"key1"},
	},
	{
		about:       "reset multiple keys",
		args:        []string{"--reset", "key1,key2,key3"},
		action:      Reset,
		keysToReset: []string{"key1", "key2", "key3"},
	},
	{
		about:      "set from file",
		args:       []string{"--file", "path"},
		action:     SetFile,
		configFile: cmd.FileVar{Path: "path"},
	},
	{
		about:       "reset resettable key",
		args:        []string{"--reset", "key1"},
		cantReset:   []string{"key2"},
		action:      Reset,
		keysToReset: []string{"key1"},
	},
	{
		about:       "reset resettable keys",
		args:        []string{"--reset", "key1,key2,key3"},
		cantReset:   []string{"key4", "key5"},
		action:      Reset,
		keysToReset: []string{"key1", "key2", "key3"},
	},
}
