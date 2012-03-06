package hook_test

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/hook"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type NilCmd struct {
	name string
}

func (c *NilCmd) Info() *cmd.Info {
	return &cmd.Info{Name: c.name}
}

func (c *NilCmd) InitFlagSet(f *gnuflag.FlagSet) {
	panic("unreachable")
}
func (c *NilCmd) ParsePositional(args []string) error {
	panic("unreachable")
}
func (c *NilCmd) Run() error {
	panic("unreachable")
}

type TestContext struct {
	charmDir string
	names    []string
	vars     map[string]string
}

func (c *TestContext) Env() *hook.Env {
	return &hook.Env{
		"client-id", "/path/to/socket", c.charmDir, "minecraft/0", c.vars,
	}
}

func (c *TestContext) Commands() []cmd.Command {
	cmds := make([]cmd.Command, len(c.names))
	for i, name := range c.names {
		cmds[i] = &NilCmd{name}
	}
	return cmds
}

type ExecSuite struct {
	origJujuc string
	toolPath  string
	outPath   string
}

var (
	_             = Suite(&ExecSuite{})
	jujucTemplate = `#!/bin/bash
echo command: $_ $* >> %s
printenv >> %s
exit 0
`
)

func (s *ExecSuite) SetUpSuite(c *C) {
	dir := c.MkDir()
	s.outPath = filepath.Join(dir, "jujuc.out")
	jujuc := fmt.Sprintf(jujucTemplate, s.outPath, s.outPath)
	s.toolPath = filepath.Join(dir, "jujuc")
	err := ioutil.WriteFile(s.toolPath, []byte(jujuc), 0755)
	c.Assert(err, IsNil)
	s.origJujuc = hook.JUJUC_PATH
	hook.JUJUC_PATH = s.toolPath
}

func (s *ExecSuite) TearDownSuite(c *C) {
	hook.JUJUC_PATH = s.origJujuc
}

func (s *ExecSuite) SetUpTest(c *C) {
	os.Remove(s.outPath)
}

func makeCharm(c *C, hookName string, perm os.FileMode, hook string) string {
	charmDir := c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, IsNil)
	hookPath := filepath.Join(hooksDir, hookName)
	err = ioutil.WriteFile(hookPath, []byte(hook), perm)
	c.Assert(err, IsNil)
	return charmDir
}

func AssertEnv(c *C, lines []string, env map[string]string) {
	for k, v := range env {
		sought := k + "=" + v
		found := false
		for i := 0; i < len(lines); i++ {
			if lines[i] == sought {
				found = true
				continue
			}
		}
		comment := Commentf("expected to find %v among %v", sought, lines)
		c.Assert(found, Equals, true, comment)
	}
}

func AssertCall(c *C, actual, expected string) {
	c.Log(actual)
	c.Log(expected)
	actuals := strings.Split(actual, " ")
	expecteds := strings.Split(expected, " ")
	c.Assert(actuals[1:], DeepEquals, expecteds[1:])
	dir, name := filepath.Split(actuals[0])
	c.Assert(name, Equals, expecteds[0])
	_, err := os.Stat(dir)
	comment := Commentf("symlinks dir not deleted")
	c.Assert(os.IsNotExist(err), Equals, true, comment)
}

func (s *ExecSuite) AssertCalls(c *C, calls []string, env map[string]string) {
	out, err := ioutil.ReadFile(s.outPath)
	if len(calls) == 0 {
		c.Assert(os.IsNotExist(err), Equals, true)
		return
	}
	c.Assert(err, IsNil)
	records := strings.Split(string(out), "command: ")
	c.Assert(records[0], Equals, "")
	records = records[1:]
	c.Assert(records, HasLen, len(calls))
	for i, record := range records {
		lines := strings.Split(record, "\n")
		AssertCall(c, lines[0], calls[i])
		AssertEnv(c, lines[1:], env)
		AssertEnv(c, lines[1:], map[string]string{
			"JUJU_CLIENT_ID":           "client-id",
			"JUJU_AGENT_SOCKET":        "/path/to/socket",
			"JUJU_UNIT_NAME":           "minecraft/0",
			"DEBIAN_FRONTEND":          "noninteractive",
			"APT_LISTCHANGES_FRONTEND": "none",
		})
	}
}

func (s *ExecSuite) TestNoHook(c *C) {
	charmDir := c.MkDir()
	ctx := &TestContext{charmDir, []string{}, map[string]string{}}
	err := hook.Exec(ctx, "tree-fell-in-forest")
	c.Assert(err, IsNil)
	s.AssertCalls(c, []string{}, map[string]string{})
}

func (s *ExecSuite) TestNonExecutableHook(c *C) {
	charmDir := makeCharm(c, "something-happened", 0600, "")
	ctx := &TestContext{charmDir, []string{}, map[string]string{}}
	err := hook.Exec(ctx, "something-happened")
	c.Assert(err, ErrorMatches, "hook is not executable: .*/something-happened")
}

func (s *ExecSuite) TestGoodHook(c *C) {
	charmDir := makeCharm(c, "something-happened", 0700, `#!/bin/bash
set -e
twiddle something
adjust something else
exit 0
`)
	ctx := &TestContext{charmDir, []string{"twiddle", "adjust"},
		map[string]string{"FOOBAR": "BAZ QUX", "BLAM": "DINK"},
	}
	err := hook.Exec(ctx, "something-happened")
	c.Assert(err, IsNil)
	s.AssertCalls(c,
		[]string{"twiddle something", "adjust something else"},
		map[string]string{"CHARM_DIR": charmDir, "FOOBAR": "BAZ QUX", "BLAM": "DINK"})
}

func (s *ExecSuite) TestBadHook(c *C) {
	charmDir := makeCharm(c, "occurrence-occurred", 0700, `#!/bin/bash
set -e
tweak malevolently
rattle with glee
exit 99
`)
	ctx := &TestContext{charmDir, []string{"rattle", "tweak"},
		map[string]string{"PEWPEW": "LASERS"}}
	err := hook.Exec(ctx, "occurrence-occurred")
	c.Assert(err, ErrorMatches, "exit status 99")
	s.AssertCalls(c,
		[]string{"tweak malevolently", "rattle with glee"},
		map[string]string{"CHARM_DIR": charmDir, "PEWPEW": "LASERS"})
}

func (s *ExecSuite) TestUnknownTool(c *C) {
	charmDir := makeCharm(c, "bewildered-caveman", 0700, `#!/bin/bash
set -e
poke exploratorily
fiddle ignorantly
thump in frustration
exit 0
`)
	ctx := &TestContext{charmDir, []string{"fiddle", "poke"},
		map[string]string{"COMPLEXITY": "OVERWHELMING"}}
	err := hook.Exec(ctx, "bewildered-caveman")
	c.Assert(err, ErrorMatches, "exit status 127")
	s.AssertCalls(c,
		[]string{"poke exploratorily", "fiddle ignorantly"},
		map[string]string{"CHARM_DIR": charmDir, "COMPLEXITY": "OVERWHELMING"})
}
