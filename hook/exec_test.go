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

type TestContext struct {
	flushed bool
	vars    map[string]string
}

func (ctx *TestContext) Vars() []string {
	result := make([]string, len(ctx.vars))
	i := 0
	for k, v := range ctx.vars {
		result[i] = k + "=" + v
		i++
	}
	return result
}

func (ctx *TestContext) Flush() error {
	ctx.flushed = true
	return nil
}

func getInfo(charmDir, remoteUnit string) *hook.ExecInfo {
	return &hook.ExecInfo{"ctx-id", "/path/to/socket", charmDir, remoteUnit}
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
		"PATH":                     os.Getenv("PATH"),
		"JUJU_CONTEXT_ID":          "ctx-id",
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
	})
}

func (s *ExecSuite) TestNoHook(c *C) {
	info := getInfo(c.MkDir(), "")
	ctx := &TestContext{}
	err := hook.Exec("tree-fell-in-forest", info, ctx)
	c.Assert(err, IsNil)
	c.Assert(ctx.flushed, Equals, false)
}

func (s *ExecSuite) TestNonExecutableHook(c *C) {
	charmDir, _ := makeCharm(c, "something-happened", 0600, 0)
	info := getInfo(charmDir, "")
	ctx := &TestContext{}
	err := hook.Exec("something-happened", info, ctx)
	c.Assert(err, ErrorMatches, `exec: ".*/something-happened": permission denied`)
	c.Assert(ctx.flushed, Equals, false)
}

func (s *ExecSuite) TestGoodHook(c *C) {
	charmDir, outPath := makeCharm(c, "something-happened", 0700, 0)
	info := getInfo(charmDir, "remote/123")
	ctx := &TestContext{vars: map[string]string{"FOOBAR": "BAZ QUX", "BLAM": "DINK"}}
	err := hook.Exec("something-happened", info, ctx)
	c.Assert(err, IsNil)
	c.Assert(ctx.flushed, Equals, true)
	s.AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR":        charmDir,
		"FOOBAR":           "BAZ QUX",
		"BLAM":             "DINK",
		"JUJU_REMOTE_UNIT": "remote/123",
	})
}

func (s *ExecSuite) TestBadHook(c *C) {
	charmDir, outPath := makeCharm(c, "occurrence-occurred", 0700, 99)
	info := getInfo(charmDir, "")
	ctx := &TestContext{vars: map[string]string{"PEWPEW": "LASERS"}}
	err := hook.Exec("occurrence-occurred", info, ctx)
	c.Assert(err, ErrorMatches, "exit status 99")
	c.Assert(ctx.flushed, Equals, false)
	s.AssertEnv(c, outPath, map[string]string{"CHARM_DIR": charmDir, "PEWPEW": "LASERS"})
}
