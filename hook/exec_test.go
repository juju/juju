package hook_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/hook"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type TestEnv struct {
	charmDir string
	vars     map[string]string
}

func (e *TestEnv) ContextId() string {
	return "ctx-id"
}

func (e *TestEnv) AgentSock() string {
	return "/path/to/socket"
}

func (e *TestEnv) UnitName() string {
	return "minecraft/0"
}

func (e *TestEnv) CharmDir() string {
	return e.charmDir
}

func (e *TestEnv) Vars() map[string]string {
	return e.vars
}

type ExecSuite struct {
	outPath string
}

var _ = Suite(&ExecSuite{})

var template = `#!/bin/bash
printenv > %s
exit %d
`

func makeCharm(c *C, hookName string, perm os.FileMode, code int) (string, string) {
	charmDir := c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, IsNil)
	hookPath := filepath.Join(hooksDir, hookName)
	outPath := filepath.Join(c.MkDir(), "hook.out")
	hook := fmt.Sprintf(template, outPath, code)
	err = ioutil.WriteFile(hookPath, []byte(hook), perm)
	c.Assert(err, IsNil)
	return charmDir, outPath
}

func AssertEnvContains(c *C, lines []string, env map[string]string) {
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
func (s *ExecSuite) AssertEnv(c *C, outPath string, env map[string]string) {
	out, err := ioutil.ReadFile(outPath)
	c.Assert(err, IsNil)
	lines := strings.Split(string(out), "\n")
	AssertEnvContains(c, lines, env)
	AssertEnvContains(c, lines, map[string]string{
		"JUJU_CONTEXT_ID":          "ctx-id",
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
		"JUJU_UNIT_NAME":           "minecraft/0",
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
	})
}

func (s *ExecSuite) TestNoHook(c *C) {
	env := &TestEnv{c.MkDir(), map[string]string{}}
	err := hook.Exec(env, "tree-fell-in-forest")
	c.Assert(err, IsNil)
}

func (s *ExecSuite) TestNonExecutableHook(c *C) {
	charmDir, _ := makeCharm(c, "something-happened", 0600, 0)
	env := &TestEnv{charmDir, map[string]string{}}
	err := hook.Exec(env, "something-happened")
	c.Assert(err, ErrorMatches, "hook is not executable: .*/something-happened")
}

func (s *ExecSuite) TestGoodHook(c *C) {
	charmDir, outPath := makeCharm(c, "something-happened", 0700, 0)
	env := &TestEnv{charmDir, map[string]string{"FOOBAR": "BAZ QUX", "BLAM": "DINK"}}
	err := hook.Exec(env, "something-happened")
	c.Assert(err, IsNil)
	s.AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR": charmDir, "FOOBAR": "BAZ QUX", "BLAM": "DINK",
	})
}

func (s *ExecSuite) TestBadHook(c *C) {
	charmDir, outPath := makeCharm(c, "occurrence-occurred", 0700, 99)
	env := &TestEnv{charmDir, map[string]string{"PEWPEW": "LASERS"}}
	err := hook.Exec(env, "occurrence-occurred")
	c.Assert(err, ErrorMatches, "exit status 99")
	s.AssertEnv(c, outPath, map[string]string{"CHARM_DIR": charmDir, "PEWPEW": "LASERS"})
}
