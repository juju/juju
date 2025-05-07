// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package cmd_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	gitjujutesting "github.com/juju/testing"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

func initDefenestrate(args []string) (*cmd.SuperCommand, *TestCommand, error) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest"})
	tc := &TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	return jc, tc, cmdtesting.InitCommand(jc, args)
}

func initDefenestrateWithAliases(c *tc.C, args []string) (*cmd.SuperCommand, *TestCommand, error) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "aliases")
	err := ioutil.WriteFile(filename, []byte(`
def = defenestrate
be-firm = defenestrate --option firmly
other = missing 
		`), 0644)
	c.Assert(err, tc.IsNil)
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest", UserAliasesFilename: filename})
	tc := &TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	return jc, tc, cmdtesting.InitCommand(jc, args)
}

type SuperCommandSuite struct {
	gitjujutesting.IsolationSuite

	ctx *cmd.Context
}

var _ = tc.Suite(&SuperCommandSuite{})

func baseSubcommandsPlus(newCommands map[string]string) map[string]string {
	subcommands := map[string]string{
		"documentation": "Generate the documentation for all commands",
		"help":          "Show help on a command or other topic.",
	}
	for name, purpose := range newCommands {
		subcommands[name] = purpose
	}
	return subcommands
}

func (s *SuperCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(s.ctx.Stderr))
}

func (s *SuperCommandSuite) TestDispatch(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest"})
	info := jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest")
	c.Assert(info.Args, tc.Equals, "<command> ...")
	c.Assert(info.Doc, tc.Equals, "")
	c.Assert(info.Subcommands, tc.DeepEquals, baseSubcommandsPlus(nil))

	jc, _, err := initDefenestrate([]string{"discombobulate"})
	c.Assert(err, tc.ErrorMatches, "unrecognized command: jujutest discombobulate")
	info = jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest")
	c.Assert(info.Args, tc.Equals, "<command> ...")
	c.Assert(info.Doc, tc.Equals, "")
	c.Assert(info.Subcommands, tc.DeepEquals, baseSubcommandsPlus(map[string]string{
		"defenestrate": "defenestrate the juju",
	}))

	jc, tc, err := initDefenestrate([]string{"defenestrate"})
	c.Assert(err, tc.IsNil)
	c.Assert(tc.Option, tc.Equals, "")
	info = jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest defenestrate")
	c.Assert(info.Args, tc.Equals, "<something>")
	c.Assert(info.Doc, tc.Equals, "defenestrate-doc")

	_, tc, err = initDefenestrate([]string{"defenestrate", "--option", "firmly"})
	c.Assert(err, tc.IsNil)
	c.Assert(tc.Option, tc.Equals, "firmly")

	_, tc, err = initDefenestrate([]string{"defenestrate", "gibberish"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["gibberish"\]`)

	// --description must be used on it's own.
	_, _, err = initDefenestrate([]string{"--description", "defenestrate"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["defenestrate"\]`)

	// --no-alias is not a valid option if there is no alias file speciifed
	_, _, err = initDefenestrate([]string{"--no-alias", "defenestrate"})
	c.Assert(err, tc.ErrorMatches, `flag provided but not defined: --no-alias`)
}

func (s *SuperCommandSuite) TestUserAliasDispatch(c *tc.C) {
	// Can still use the full name.
	jc, tc, err := initDefenestrateWithAliases(c, []string{"defenestrate"})
	c.Assert(err, tc.IsNil)
	c.Assert(tc.Option, tc.Equals, "")
	info := jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest defenestrate")
	c.Assert(info.Args, tc.Equals, "<something>")
	c.Assert(info.Doc, tc.Equals, "defenestrate-doc")

	jc, tc, err = initDefenestrateWithAliases(c, []string{"def"})
	c.Assert(err, tc.IsNil)
	c.Assert(tc.Option, tc.Equals, "")
	info = jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest defenestrate")

	jc, tc, err = initDefenestrateWithAliases(c, []string{"be-firm"})
	c.Assert(err, tc.IsNil)
	c.Assert(tc.Option, tc.Equals, "firmly")
	info = jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest defenestrate")

	_, _, err = initDefenestrateWithAliases(c, []string{"--no-alias", "def"})
	c.Assert(err, tc.ErrorMatches, "unrecognized command: jujutest def")

	// Aliases to missing values are converted before lookup.
	_, _, err = initDefenestrateWithAliases(c, []string{"other"})
	c.Assert(err, tc.ErrorMatches, "unrecognized command: jujutest missing")
}

func (s *SuperCommandSuite) TestRegister(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest"})
	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flap"})
	badCall := func() { jc.Register(&TestCommand{Name: "flap"}) }
	c.Assert(badCall, tc.PanicMatches, `command already registered: "flap"`)
}

func (s *SuperCommandSuite) TestAliasesRegistered(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest"})
	jc.Register(&TestCommand{Name: "flip", Aliases: []string{"flap", "flop"}})

	info := jc.Info()
	c.Assert(info.Subcommands, tc.DeepEquals, baseSubcommandsPlus(map[string]string{
		"flap": "Alias for 'flip'.",
		"flip": "flip the juju",
		"flop": "Alias for 'flip'.",
	}))
}

func (s *SuperCommandSuite) TestInfo(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "jujutest",
		Purpose: "to be purposeful",
		Doc:     "doc\nblah\ndoc",
	})
	info := jc.Info()
	c.Assert(info.Name, tc.Equals, "jujutest")
	c.Assert(info.Purpose, tc.Equals, "to be purposeful")
	c.Assert(info.Doc, tc.Matches, jc.Doc)
	c.Assert(info.Subcommands, tc.DeepEquals, baseSubcommandsPlus(nil))

	subcommands := baseSubcommandsPlus(map[string]string{
		"flapbabble": "flapbabble the juju",
		"flip":       "flip the juju",
	})
	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flapbabble"})
	info = jc.Info()
	c.Assert(info.Doc, tc.Matches, jc.Doc)
	c.Assert(info.Subcommands, tc.DeepEquals, subcommands)

	jc.Doc = ""
	info = jc.Info()
	c.Assert(info.Doc, tc.Equals, "")
	c.Assert(info.Subcommands, tc.DeepEquals, subcommands)
}

type testVersionFlagCommand struct {
	cmd.CommandBase
	version string
}

func (c *testVersionFlagCommand) Info() *cmd.Info {
	return &cmd.Info{Name: "test"}
}

func (c *testVersionFlagCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.version, "version", "", "")
}

func (c *testVersionFlagCommand) Run(_ *cmd.Context) error {
	return nil
}

func (s *SuperCommandSuite) TestVersionVerb(c *tc.C) {
	s.testVersion(c, []string{"version"})
}

func (s *SuperCommandSuite) TestVersionFlag(c *tc.C) {
	s.testVersion(c, []string{"--version"})
}

func (s *SuperCommandSuite) testVersion(c *tc.C, params []string) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "jujutest",
		Purpose: "to be purposeful",
		Doc:     "doc\nblah\ndoc",
		Version: "111.222.333",
	})
	testVersionFlagCommand := &testVersionFlagCommand{}
	jc.Register(testVersionFlagCommand)

	code := cmd.Main(jc, s.ctx, params)
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "111.222.333\n")
}

func (s *SuperCommandSuite) TestVersionFlagSpecific(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "jujutest",
		Purpose: "to be purposeful",
		Doc:     "doc\nblah\ndoc",
		Version: "111.222.333",
	})
	testVersionFlagCommand := &testVersionFlagCommand{}
	jc.Register(testVersionFlagCommand)

	// juju test --version should update testVersionFlagCommand.version,
	// and there should be no output. The --version flag on the 'test'
	// subcommand has a different type to the "juju --version" flag.
	code := cmd.Main(jc, s.ctx, []string{"test", "--version=abc.123"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "")
	c.Assert(testVersionFlagCommand.version, tc.Equals, "abc.123")
}

func (s *SuperCommandSuite) TestVersionNotProvidedVerb(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "jujutest",
		Purpose: "to be purposeful",
		Doc:     "doc\nblah\ndoc",
	})
	// juju version
	code := cmd.Main(jc, s.ctx, []string{"version"})
	c.Check(code, tc.Not(tc.Equals), 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "ERROR unrecognized command: jujutest version\n")
}

func (s *SuperCommandSuite) TestVersionNotProvidedFlag(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "jujutest",
		Purpose: "to be purposeful",
		Doc:     "doc\nblah\ndoc",
	})
	// juju --version
	code := cmd.Main(jc, s.ctx, []string{"--version"})
	c.Check(code, tc.Not(tc.Equals), 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "ERROR flag provided but not defined: --version\n")
}

func (s *SuperCommandSuite) TestVersionNotProvidedOption(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "jujutest",
		Purpose: "to be purposeful",
		Doc:     "doc\nblah\ndoc",
	})
	// juju --version where flags are known as options
	jc.FlagKnownAs = "option"
	code := cmd.Main(jc, s.ctx, []string{"--version"})
	c.Check(code, tc.Not(tc.Equals), 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "ERROR option provided but not defined: --version\n")
}

func (s *SuperCommandSuite) TestLogging(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	sc.Register(&TestCommand{Name: "blah"})
	code := cmd.Main(sc, s.ctx, []string{"blah", "--option", "error", "--debug"})
	c.Assert(code, tc.Equals, 1)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Matches, `(?m)ERROR BAM!\n.* DEBUG .* error stack: \n.*`)
}

type notifyTest struct {
	usagePrefix string
	name        string
	expectName  string
}

func (s *SuperCommandSuite) TestNotifyRunJujuJuju(c *tc.C) {
	s.testNotifyRun(c, notifyTest{"juju", "juju", "juju"})
}
func (s *SuperCommandSuite) TestNotifyRunSomethingElse(c *tc.C) {
	s.testNotifyRun(c, notifyTest{"something", "else", "something else"})
}
func (s *SuperCommandSuite) TestNotifyRunJuju(c *tc.C) {
	s.testNotifyRun(c, notifyTest{"", "juju", "juju"})
}
func (s *SuperCommandSuite) TestNotifyRunMyApp(c *tc.C) {
	s.testNotifyRun(c, notifyTest{"", "myapp", "myapp"})
}

func (s *SuperCommandSuite) testNotifyRun(c *tc.C, test notifyTest) {
	notifyName := ""
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: test.usagePrefix,
		Name:        test.name,
		NotifyRun: func(name string) {
			notifyName = name
		},
		Log: &cmd.Log{},
	})
	sc.Register(&TestCommand{Name: "blah"})
	code := cmd.Main(sc, s.ctx, []string{"blah", "--option", "error"})
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Matches, "ERROR BAM!\n")
	c.Assert(code, tc.Equals, 1)
	c.Assert(notifyName, tc.Equals, test.expectName)
}

func (s *SuperCommandSuite) TestDescription(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest", Purpose: "blow up the death star"})
	jc.Register(&TestCommand{Name: "blah"})
	code := cmd.Main(jc, s.ctx, []string{"blah", "--description"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "blow up the death star\n")
}

func NewSuperWithCallback(callback func(*cmd.Context, string, []string) error) cmd.Command {
	return cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "jujutest",
		Log:             &cmd.Log{},
		MissingCallback: callback,
	})
}

func (s *SuperCommandSuite) TestMissingCallback(c *tc.C) {
	var calledName string
	var calledArgs []string

	callback := func(ctx *cmd.Context, subcommand string, args []string) error {
		calledName = subcommand
		calledArgs = args
		return nil
	}

	code := cmd.Main(
		NewSuperWithCallback(callback),
		cmdtesting.Context(c),
		[]string{"foo", "bar", "baz", "--debug"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(calledName, tc.Equals, "foo")
	c.Assert(calledArgs, tc.DeepEquals, []string{"bar", "baz", "--debug"})
}

func (s *SuperCommandSuite) TestMissingCallbackErrors(c *tc.C) {
	callback := func(ctx *cmd.Context, subcommand string, args []string) error {
		return fmt.Errorf("command not found %q", subcommand)
	}

	code := cmd.Main(NewSuperWithCallback(callback), s.ctx, []string{"foo"})
	c.Assert(code, tc.Equals, 1)
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "ERROR command not found \"foo\"\n")
}

func (s *SuperCommandSuite) TestMissingCallbackContextWiredIn(c *tc.C) {
	callback := func(ctx *cmd.Context, subcommand string, args []string) error {
		fmt.Fprintf(ctx.Stdout, "this is std out")
		fmt.Fprintf(ctx.Stderr, "this is std err")
		return nil
	}

	code := cmd.Main(NewSuperWithCallback(callback), s.ctx, []string{"foo", "bar", "baz", "--debug"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "this is std out")
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "this is std err")
}

type simpleWithInitError struct {
	cmd.CommandBase
	name      string
	initError error
}

var _ cmd.Command = (*simpleWithInitError)(nil)

func (s *simpleWithInitError) Info() *cmd.Info {
	return &cmd.Info{Name: s.name, Purpose: "to be simple"}
}

func (s *simpleWithInitError) Init(args []string) error {
	return s.initError
}

func (s *simpleWithInitError) Run(_ *cmd.Context) error {
	return errors.New("unexpected-error")
}

func (s *SuperCommandSuite) TestMissingCallbackSetOnError(c *tc.C) {
	callback := func(ctx *cmd.Context, subcommand string, args []string) error {
		fmt.Fprint(ctx.Stdout, "reached callback: "+strings.Join(args, " "))
		return nil
	}

	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "jujutest",
		Log:             &cmd.Log{},
		MissingCallback: callback,
	})
	jc.Register(&simpleWithInitError{name: "foo", initError: cmd.ErrCommandMissing})
	jc.Register(&simpleWithInitError{name: "bar", initError: errors.New("my-fake-error")})

	code := cmd.Main(jc, s.ctx, []string{"bar"})
	c.Assert(code, tc.Equals, 2)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "ERROR my-fake-error\n")

	// Verify that a call to foo, which returns a ErrCommandMissing error
	// triggers the command missing callback and ensure all expected
	// args were correctly sent to the callback.
	code = cmd.Main(jc, s.ctx, []string{"foo", "bar", "baz", "--debug"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "reached callback: bar baz --debug")
}

func (s *SuperCommandSuite) TestSupercommandAliases(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "jujutest",
		UsagePrefix: "juju",
	})
	sub := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "jubar",
		UsagePrefix: "juju jujutest",
		Aliases:     []string{"jubaz", "jubing"},
	})
	info := sub.Info()
	c.Check(info.Aliases, tc.DeepEquals, []string{"jubaz", "jubing"})
	jc.Register(sub)
	for _, name := range []string{"jubar", "jubaz", "jubing"} {
		c.Logf("testing command name %q", name)
		s.SetUpTest(c)
		code := cmd.Main(jc, s.ctx, []string{name, "--help"})
		c.Assert(code, tc.Equals, 0)
		c.Assert(cmdtesting.Stdout(s.ctx), tc.Matches, "(?s).*Usage: juju jujutest jubar.*")
		c.Assert(cmdtesting.Stdout(s.ctx), tc.Matches, "(?s).*Aliases: jubaz, jubing.*")
		s.TearDownTest(c)
	}
}

type simple struct {
	cmd.CommandBase
	name string
	args []string
}

var _ cmd.Command = (*simple)(nil)

func (s *simple) Info() *cmd.Info {
	return &cmd.Info{Name: s.name, Purpose: "to be simple"}
}

func (s *simple) Init(args []string) error {
	s.args = args
	return nil
}

func (s *simple) Run(ctx *cmd.Context) error {
	fmt.Fprintf(ctx.Stdout, "%s %s\n", s.name, strings.Join(s.args, ", "))
	return nil
}

type deprecate struct {
	replacement string
	obsolete    bool
}

func (d deprecate) Deprecated() (bool, string) {
	if d.replacement == "" {
		return false, ""
	}
	return true, d.replacement
}
func (d deprecate) Obsolete() bool {
	return d.obsolete
}

func (s *SuperCommandSuite) TestRegisterAlias(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujutest",
	})
	jc.Register(&simple{name: "test"})
	jc.RegisterAlias("foo", "test", nil)
	jc.RegisterAlias("bar", "test", deprecate{replacement: "test"})
	jc.RegisterAlias("baz", "test", deprecate{obsolete: true})

	c.Assert(
		func() { jc.RegisterAlias("omg", "unknown", nil) },
		tc.PanicMatches, `"unknown" not found when registering alias`)

	info := jc.Info()
	// NOTE: deprecated `bar` not shown in commands.
	c.Assert(info.Doc, tc.Equals, "")
	c.Assert(info.Subcommands, tc.DeepEquals, baseSubcommandsPlus(map[string]string{
		"foo":  "Alias for 'test'.",
		"test": "to be simple",
	}))

	for _, test := range []struct {
		name   string
		stdout string
		stderr string
		code   int
	}{
		{
			name:   "test",
			stdout: "test arg\n",
		}, {
			name:   "foo",
			stdout: "test arg\n",
		}, {
			name:   "bar",
			stdout: "test arg\n",
			stderr: "WARNING \"bar\" is deprecated, please use \"test\"\n",
		}, {
			name:   "baz",
			stderr: "ERROR unrecognized command: jujutest baz\n",
			code:   2,
		},
	} {
		s.SetUpTest(c)
		code := cmd.Main(jc, s.ctx, []string{test.name, "arg"})
		c.Check(code, tc.Equals, test.code)
		c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, test.stdout)
		c.Check(cmdtesting.Stderr(s.ctx), tc.Equals, test.stderr)
		s.TearDownTest(c)
	}
}

func (s *SuperCommandSuite) TestRegisterSuperAlias(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujutest",
	})
	jc.Register(&simple{name: "test"})
	sub := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "bar",
		UsagePrefix: "jujutest",
		Purpose:     "bar functions",
	})
	jc.Register(sub)
	sub.Register(&simple{name: "foo"})

	c.Assert(
		func() { jc.RegisterSuperAlias("bar-foo", "unknown", "foo", nil) },
		tc.PanicMatches, `"unknown" not found when registering alias`)
	c.Assert(
		func() { jc.RegisterSuperAlias("bar-foo", "test", "foo", nil) },
		tc.PanicMatches, `"test" is not a SuperCommand`)
	c.Assert(
		func() { jc.RegisterSuperAlias("bar-foo", "bar", "unknown", nil) },
		tc.PanicMatches, `"unknown" not found as a command in "bar"`)

	jc.RegisterSuperAlias("bar-foo", "bar", "foo", nil)
	jc.RegisterSuperAlias("bar-dep", "bar", "foo", deprecate{replacement: "bar foo"})
	jc.RegisterSuperAlias("bar-ob", "bar", "foo", deprecate{obsolete: true})

	info := jc.Info()
	// NOTE: deprecated `bar` not shown in commands.
	c.Assert(info.Subcommands, tc.DeepEquals, baseSubcommandsPlus(map[string]string{
		"bar":     "bar functions",
		"bar-foo": "Alias for 'bar foo'.",
		"test":    "to be simple",
	}))

	for _, test := range []struct {
		args   []string
		stdout string
		stderr string
		code   int
	}{
		{
			args:   []string{"bar", "foo", "arg"},
			stdout: "foo arg\n",
		}, {
			args:   []string{"bar-foo", "arg"},
			stdout: "foo arg\n",
		}, {
			args:   []string{"bar-dep", "arg"},
			stdout: "foo arg\n",
			stderr: "WARNING \"bar-dep\" is deprecated, please use \"bar foo\"\n",
		}, {
			args:   []string{"bar-ob", "arg"},
			stderr: "ERROR unrecognized command: jujutest bar-ob\n",
			code:   2,
		},
	} {
		s.SetUpTest(c)
		code := cmd.Main(jc, s.ctx, test.args)
		c.Check(code, tc.Equals, test.code)
		c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, test.stdout)
		c.Check(cmdtesting.Stderr(s.ctx), tc.Equals, test.stderr)
		s.TearDownTest(c)
	}
}

type simpleAlias struct {
	simple
}

func (s *simpleAlias) Info() *cmd.Info {
	return &cmd.Info{Name: s.name, Purpose: "to be simple with an alias",
		Aliases: []string{s.name + "-alias"}}
}

func (s *SuperCommandSuite) TestRegisterDeprecated(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujutest",
	})

	// Test that calling with a nil command will not panic
	jc.RegisterDeprecated(nil, nil)

	jc.RegisterDeprecated(&simpleAlias{simple{name: "test-non-dep"}}, nil)
	jc.RegisterDeprecated(&simpleAlias{simple{name: "test-dep"}}, deprecate{replacement: "test-dep-new"})
	jc.RegisterDeprecated(&simpleAlias{simple{name: "test-ob"}}, deprecate{obsolete: true})

	badCall := func() {
		jc.RegisterDeprecated(&simpleAlias{simple{name: "test-dep"}}, deprecate{replacement: "test-dep-new"})
	}
	c.Assert(badCall, tc.PanicMatches, `command already registered: "test-dep"`)

	for _, test := range []struct {
		args   []string
		stdout string
		stderr string
		code   int
	}{
		{
			args:   []string{"test-non-dep", "arg"},
			stdout: "test-non-dep arg\n",
		}, {
			args:   []string{"test-non-dep-alias", "arg"},
			stdout: "test-non-dep arg\n",
		}, {
			args:   []string{"test-dep", "arg"},
			stdout: "test-dep arg\n",
			stderr: "WARNING \"test-dep\" is deprecated, please use \"test-dep-new\"\n",
		}, {
			args:   []string{"test-dep-alias", "arg"},
			stdout: "test-dep arg\n",
			stderr: "WARNING \"test-dep-alias\" is deprecated, please use \"test-dep-new\"\n",
		}, {
			args:   []string{"test-ob", "arg"},
			stderr: "ERROR unrecognized command: jujutest test-ob\n",
			code:   2,
		}, {
			args:   []string{"test-ob-alias", "arg"},
			stderr: "ERROR unrecognized command: jujutest test-ob-alias\n",
			code:   2,
		},
	} {
		s.SetUpTest(c)
		code := cmd.Main(jc, s.ctx, test.args)
		c.Check(code, tc.Equals, test.code)
		c.Check(cmdtesting.Stderr(s.ctx), tc.Equals, test.stderr)
		c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, test.stdout)
		s.TearDownTest(c)
	}
}

func (s *SuperCommandSuite) TestGlobalFlagsBeforeCommand(c *tc.C) {
	flag := ""
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		GlobalFlags: flagAdderFunc(func(fset *gnuflag.FlagSet) {
			fset.StringVar(&flag, "testflag", "", "global test flag")
		}),
		Log: &cmd.Log{},
	})
	sc.Register(&TestCommand{Name: "blah"})
	code := cmd.Main(sc, s.ctx, []string{
		"--testflag=something",
		"blah",
		"--option=testoption",
	})
	c.Assert(code, tc.Equals, 0)
	c.Assert(flag, tc.Equals, "something")
	c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, "testoption\n")
}

func (s *SuperCommandSuite) TestGlobalFlagsAfterCommand(c *tc.C) {
	flag := ""
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		GlobalFlags: flagAdderFunc(func(fset *gnuflag.FlagSet) {
			fset.StringVar(&flag, "testflag", "", "global test flag")
		}),
		Log: &cmd.Log{},
	})
	sc.Register(&TestCommand{Name: "blah"})
	code := cmd.Main(sc, s.ctx, []string{
		"blah",
		"--option=testoption",
		"--testflag=something",
	})
	c.Assert(code, tc.Equals, 0)
	c.Assert(flag, tc.Equals, "something")
	c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, "testoption\n")
}

func (s *SuperCommandSuite) TestSuperSetFlags(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
		FlagKnownAs: "option",
	})
	s.assertFlagsAlias(c, sc, "option")
}

func (s *SuperCommandSuite) TestSuperSetFlagsDefault(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	s.assertFlagsAlias(c, sc, "flag")
}

func (s *SuperCommandSuite) assertFlagsAlias(c *tc.C, sc *cmd.SuperCommand, expectedAlias string) {
	sc.Register(&TestCommand{Name: "blah"})
	code := cmd.Main(sc, s.ctx, []string{
		"blah",
		"--fluffs",
	})
	c.Assert(code, tc.Equals, 2)
	c.Check(s.ctx.IsSerial(), tc.Equals, false)
	c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, "")
	c.Check(cmdtesting.Stderr(s.ctx), tc.Equals, fmt.Sprintf("ERROR %v provided but not defined: --fluffs\n", expectedAlias))
}

func (s *SuperCommandSuite) TestErrInJson(c *tc.C) {
	output := cmd.Output{}
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
		GlobalFlags: flagAdderFunc(func(fset *gnuflag.FlagSet) {
			output.AddFlags(fset, "json", map[string]cmd.Formatter{"json": cmd.FormatJson})
		}),
	})
	s.assertFormattingErr(c, sc, "json")
}

func (s *SuperCommandSuite) TestErrInYaml(c *tc.C) {
	output := cmd.Output{}
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
		GlobalFlags: flagAdderFunc(func(fset *gnuflag.FlagSet) {
			output.AddFlags(fset, "yaml", map[string]cmd.Formatter{"yaml": cmd.FormatYaml})
		}),
	})
	s.assertFormattingErr(c, sc, "yaml")
}

func (s *SuperCommandSuite) TestErrInJsonWithOutput(c *tc.C) {
	output := cmd.Output{}
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
		GlobalFlags: flagAdderFunc(func(fset *gnuflag.FlagSet) {
			output.AddFlags(fset, "json", map[string]cmd.Formatter{"json": cmd.FormatJson})
		}),
	})
	// This command will throw an error during the run after logging a structured output.
	testCmd := &TestCommand{
		Name:   "blah",
		Option: "error",
		CustomRun: func(ctx *cmd.Context) error {
			output.Write(ctx, struct {
				Name string `json:"name"`
			}{Name: "test"})
			return errors.New("BAM!")
		},
	}
	sc.Register(testCmd)
	code := cmd.Main(sc, s.ctx, []string{
		"blah",
		"--format=json",
		"--option=error",
	})
	c.Assert(code, tc.Equals, 1)
	c.Check(s.ctx.IsSerial(), tc.Equals, true)
	c.Check(cmdtesting.Stderr(s.ctx), tc.Matches, "ERROR BAM!\n")
	c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, "{\"name\":\"test\"}\n")
}

func (s *SuperCommandSuite) assertFormattingErr(c *tc.C, sc *cmd.SuperCommand, format string) {
	// This command will throw an error during the run
	testCmd := &TestCommand{Name: "blah", Option: "error"}
	sc.Register(testCmd)
	formatting := fmt.Sprintf("--format=%v", format)
	code := cmd.Main(sc, s.ctx, []string{
		"blah",
		formatting,
		"--option=error",
	})
	c.Assert(code, tc.Equals, 1)
	c.Check(s.ctx.IsSerial(), tc.Equals, true)
	c.Check(cmdtesting.Stderr(s.ctx), tc.Matches, "ERROR BAM!\n")
	c.Check(cmdtesting.Stdout(s.ctx), tc.Equals, "{}\n")
}

type flagAdderFunc func(*gnuflag.FlagSet)

func (f flagAdderFunc) AddFlags(fset *gnuflag.FlagSet) {
	f(fset)
}

func (s *SuperCommandSuite) TestFindClosestSubCommand(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	name, _, ok := sc.FindClosestSubCommand("halp") //nolint:misspell
	c.Assert(ok, tc.Equals, true)
	c.Assert(name, tc.Equals, "help")
}

func (s *SuperCommandSuite) TestFindClosestSubCommandReturnsExactMatch(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	name, _, ok := sc.FindClosestSubCommand("help")
	c.Assert(ok, tc.Equals, true)
	c.Assert(name, tc.Equals, "help")
}

func (s *SuperCommandSuite) TestFindClosestSubCommandReturnsNonExactMatch(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	_, _, ok := sc.FindClosestSubCommand("sillycommand")
	c.Assert(ok, tc.Equals, false)
}

func (s *SuperCommandSuite) TestFindClosestSubCommandReturnsWithPartialName(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	name, _, ok := sc.FindClosestSubCommand("hel")
	c.Assert(ok, tc.Equals, true)
	c.Assert(name, tc.Equals, "help")
}

func (s *SuperCommandSuite) TestFindClosestSubCommandReturnsWithLessMisspeltName(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	name, _, ok := sc.FindClosestSubCommand("hlp")
	c.Assert(ok, tc.Equals, true)
	c.Assert(name, tc.Equals, "help")
}

func (s *SuperCommandSuite) TestFindClosestSubCommandReturnsWithMoreName(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	name, _, ok := sc.FindClosestSubCommand("helper")
	c.Assert(ok, tc.Equals, true)
	c.Assert(name, tc.Equals, "help")
}

func (s *SuperCommandSuite) TestFindClosestSubCommandReturnsConsistentResults(c *tc.C) {
	sc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "juju",
		Name:        "command",
		Log:         &cmd.Log{},
	})
	sc.Register(cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "hxlp",
		Name:        "hxlp",
		Log:         &cmd.Log{},
	}))
	sc.Register(cmd.NewSuperCommand(cmd.SuperCommandParams{
		UsagePrefix: "hflp",
		Name:        "hflp",
		Log:         &cmd.Log{},
	}))
	name, _, ok := sc.FindClosestSubCommand("helper")
	c.Assert(ok, tc.Equals, true)
	c.Assert(name, tc.Equals, "help")
}
